package gcp

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
	"google.golang.org/api/iterator"
)

// InstanceOperationsInterface defines the high-level instance operations interface
type InstanceOperationsInterface interface {
	StartInstance(ctx context.Context, projectID, zone, instanceName string) error
	StopInstance(ctx context.Context, projectID, zone, instanceName string) error
	ListInstancesInZone(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error)
	AggregatedListInstances(ctx context.Context, projectID string) ([]*computepb.Instance, error)
	GetInstance(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error)
	InstanceIsRunning(ctx context.Context, instance *computepb.Instance) bool
	GetInstanceDisks(ctx context.Context, projectID, zone, instanceName string) ([]*computepb.AttachedDisk, error)
	DeleteInstance(ctx context.Context, projectID, zone, instanceName string) error
	AttachDisk(ctx context.Context, projectID, zone, instanceName string, attachedDiskResource *computepb.AttachedDisk) error
	DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) error
}

// InstanceClient wraps the GCP instances client and provides instance operation methods
type InstanceClient struct {
	client GceClientInterface
}

// NewInstanceClient creates a new InstanceClient with the provided GceClientInterface
func NewInstanceClient(client GceClientInterface) *InstanceClient {
	return &InstanceClient{
		client: client,
	}
}

func (ic *InstanceClient) StartInstance(ctx context.Context, projectID, zone, instanceName string) error {
	logFields := map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}
	logger.Op.WithFields(logFields).Info("Starting instance")

	req := &computepb.StartInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ic.client.Start(ctx, req)

	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to start instance")
		return fmt.Errorf("failed to start instance %s in zone %s: %w", instanceName, zone, err)
	}
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for operation %s failed", op.Name())
		return fmt.Errorf("waiting for instance %s start operation failed: %w", instanceName, err)
	}

	logger.Op.WithFields(logFields).Info("Instance started successfully.")
	return nil
}

func (ic *InstanceClient) StopInstance(ctx context.Context, projectID, zone, instanceName string) error {
	logFields := map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}
	logger.Op.WithFields(logFields).Info("Stopping instance")
	zone = utils.ExtractZoneName(zone)
	req := &computepb.StopInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ic.client.Stop(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to initiate instance stop operation")
		return fmt.Errorf("failed to stop instance %s in zone %s: %w", instanceName, zone, err)
	}

	logger.Op.WithFields(logFields).Infof("Waiting for instance stop operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for instance stop operation %s failed", op.Name())
		return fmt.Errorf("waiting for instance %s stop operation failed: %w", instanceName, err)
	}

	logger.Op.WithFields(logFields).Info("Instance stopped successfully.")
	return nil
}

func (ic *InstanceClient) ListInstancesInZone(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error) {

	req := &computepb.ListInstancesRequest{
		Project: projectID,
		Zone:    zone,
	}
	it := ic.client.List(ctx, req)
	var instances []*computepb.Instance
	for {
		instance, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate instances in zone %s: %w", zone, err)
		}
		instances = append(instances, instance)
	}
	logger.Op.WithFields(map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instaces": len(instances),
	}).Info("Instances found")
	return instances, nil
}

func (ic *InstanceClient) AggregatedListInstances(ctx context.Context, projectID string) ([]*computepb.Instance, error) {
	logger.Op.WithFields(map[string]interface{}{
		"project": projectID,
	}).Info("Listing all compute instances in project (aggregated list)")

	req := &computepb.AggregatedListInstancesRequest{
		Project: projectID,
	}
	it := ic.client.AggregatedList(ctx, req)
	var instances []*computepb.Instance
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate aggregated instances for project %s: %w", projectID, err)
		}
		if pair.Value != nil && pair.Value.Instances != nil {
			instances = append(instances, pair.Value.Instances...)
		}
	}
	logger.Op.WithFields(map[string]interface{}{
		"project": projectID,
		"count":   len(instances),
	}).Info("Successfully listed all instances in project.")
	return instances, nil
}

func (ic *InstanceClient) GetInstance(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error) {
	logger.Op.WithFields(map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Getting instance details")

	req := &computepb.GetInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	instance, err := ic.client.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, zone, err)
	}
	return instance, nil
}

func (ic *InstanceClient) InstanceIsRunning(ctx context.Context, instance *computepb.Instance) bool {

	return *instance.Status == "RUNNING"
}

func (ic *InstanceClient) GetInstanceDisks(ctx context.Context, projectID, zone, instanceName string) ([]*computepb.AttachedDisk, error) {
	logger.Op.WithFields(map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Getting attached disks for instance")

	instance, err := ic.GetInstance(ctx, projectID, zone, instanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, zone, err)
	}

	if instance.Disks == nil {
		return nil, fmt.Errorf("no disks found for instance %s in zone %s", instanceName, zone)
	}

	return instance.Disks, nil
}

func (ic *InstanceClient) DeleteInstance(ctx context.Context, projectID, zone, instanceName string) error {
	logFields := map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}
	logger.Op.WithFields(logFields).Info("Deleting instance")

	req := &computepb.DeleteInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ic.client.Delete(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to initiate instance delete operation")
		return fmt.Errorf("failed to delete instance %s in zone %s: %w", instanceName, zone, err)
	}

	logger.Op.WithFields(logFields).Infof("Waiting for instance delete operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for instance delete operation %s failed", op.Name())
		return fmt.Errorf("waiting for instance %s delete operation failed: %w", instanceName, err)
	}
	logger.Op.WithFields(logFields).Info("Instance deleted successfully.")
	return nil
}

func (ic *InstanceClient) AttachDisk(ctx context.Context, projectID, zone, instanceName string, attachedDiskResource *computepb.AttachedDisk) error {
	logFields := map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
		"disk":     attachedDiskResource.GetSource(),
	}
	logger.Op.WithFields(logFields).Info("Attaching disk to instance")

	req := &computepb.AttachDiskInstanceRequest{
		Project:              projectID,
		Zone:                 zone,
		Instance:             instanceName,
		AttachedDiskResource: attachedDiskResource,
	}

	op, err := ic.client.AttachDisk(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to initiate attach disk operation")
		return fmt.Errorf("failed to attach disk to instance %s in zone %s: %w", instanceName, zone, err)
	}

	logger.Op.WithFields(logFields).Infof("Waiting for attach disk operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for attach disk operation %s failed", op.Name())
		return fmt.Errorf("waiting for attach disk to instance %s operation failed: %w", instanceName, err)
	}

	logger.Op.WithFields(logFields).Info("Disk attached successfully to instance.")
	return nil
}

// DetachDisk detaches a disk from a GCE instance.
func (ic *InstanceClient) DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) error {
	logFields := map[string]interface{}{
		"project":    projectID,
		"zone":       zone,
		"instance":   instanceName,
		"deviceName": deviceName,
	}
	logger.Op.WithFields(logFields).Info("Detaching disk from instance")

	req := &computepb.DetachDiskInstanceRequest{
		Project:    projectID,
		Zone:       zone,
		Instance:   instanceName,
		DeviceName: deviceName,
	}

	op, err := ic.client.DetachDisk(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to initiate detach disk operation")
		return fmt.Errorf("failed to detach disk %s from instance %s in zone %s: %w", deviceName, instanceName, zone, err)
	}
	logger.Op.WithFields(logFields).Infof("Waiting for detach disk operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for detach disk operation %s failed", op.Name())
		return fmt.Errorf("waiting for detach disk %s from instance %s operation failed: %w", deviceName, instanceName, err)
	}

	logger.Op.WithFields(logFields).Info("Disk detached successfully from instance.")
	return nil
}