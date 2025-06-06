package gcp

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/utils"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

func (ops *Clients) StartInstance(ctx context.Context, projectID, zone, instanceName string) error {
	logFields := logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}
	logrus.WithFields(logFields).Info("Starting instance")

	req := &computepb.StartInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ops.Gce.Start(ctx, req)

	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to start instance")
		return fmt.Errorf("failed to start instance %s in zone %s: %w", instanceName, zone, err)
	}
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for operation %s failed", op.Name())
		return fmt.Errorf("waiting for instance %s start operation failed: %w", instanceName, err)
	}

	logrus.WithFields(logFields).Info("Instance started successfully.")
	return nil
}

func (ops *Clients) StopInstance(ctx context.Context, projectID, zone, instanceName string) error {
	logFields := logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}
	logrus.WithFields(logFields).Info("Stopping instance")
	zone = utils.ExtractZoneName(zone)
	req := &computepb.StopInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ops.Gce.Stop(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate instance stop operation")
		return fmt.Errorf("failed to stop instance %s in zone %s: %w", instanceName, zone, err)
	}

	logrus.WithFields(logFields).Infof("Waiting for instance stop operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for instance stop operation %s failed", op.Name())
		return fmt.Errorf("waiting for instance %s stop operation failed: %w", instanceName, err)
	}

	logrus.WithFields(logFields).Info("Instance stopped successfully.")
	return nil
}

func (ops *Clients) ListInstancesInZone(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error) {

	req := &computepb.ListInstancesRequest{
		Project: projectID,
		Zone:    zone,
	}
	it := ops.Gce.List(ctx, req)
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
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instaces": len(instances),
	}).Info("Instances found")
	return instances, nil
}

func (ops *Clients) AggregatedListInstances(ctx context.Context, projectID string) ([]*computepb.Instance, error) {
	logrus.WithFields(logrus.Fields{
		"project": projectID,
	}).Info("Listing all compute instances in project (aggregated list)")

	req := &computepb.AggregatedListInstancesRequest{
		Project: projectID,
	}
	it := ops.Gce.AggregatedList(ctx, req)
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
	logrus.WithFields(logrus.Fields{
		"project": projectID,
		"count":   len(instances),
	}).Info("Successfully listed all instances in project.")
	return instances, nil
}

func (ops *Clients) GetInstance(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error) {
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Getting instance details")

	req := &computepb.GetInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	instance, err := ops.Gce.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, zone, err)
	}
	return instance, nil
}

func (ops *Clients) InstanceIsRunning(ctx context.Context, instance *computepb.Instance) bool {

	return *instance.Status == "RUNNING"
}

func (ops *Clients) GetInstanceDisks(ctx context.Context, projectID, zone, instanceName string) ([]*computepb.AttachedDisk, error) {
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Getting attached disks for instance")

	instance, err := ops.GetInstance(ctx, projectID, zone, instanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, zone, err)
	}

	if instance.Disks == nil {
		return nil, fmt.Errorf("no disks found for instance %s in zone %s", instanceName, zone)
	}

	return instance.Disks, nil
}

func (ops *Clients) DeleteInstance(ctx context.Context, projectID, zone, instanceName string) error {
	logFields := logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}
	logrus.WithFields(logFields).Info("Deleting instance")

	req := &computepb.DeleteInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ops.Gce.Delete(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate instance delete operation")
		return fmt.Errorf("failed to delete instance %s in zone %s: %w", instanceName, zone, err)
	}

	logrus.WithFields(logFields).Infof("Waiting for instance delete operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for instance delete operation %s failed", op.Name())
		return fmt.Errorf("waiting for instance %s delete operation failed: %w", instanceName, err)
	}
	logrus.WithFields(logFields).Info("Instance deleted successfully.")
	return nil
}

func (ops *Clients) AttachDisk(ctx context.Context, projectID, zone, instanceName string, attachedDiskResource *computepb.AttachedDisk) error {
	logFields := logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
		"disk":     attachedDiskResource.GetSource(),
	}
	logrus.WithFields(logFields).Info("Attaching disk to instance")

	req := &computepb.AttachDiskInstanceRequest{
		Project:              projectID,
		Zone:                 zone,
		Instance:             instanceName,
		AttachedDiskResource: attachedDiskResource,
	}

	op, err := ops.Gce.AttachDisk(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate attach disk operation")
		return fmt.Errorf("failed to attach disk to instance %s in zone %s: %w", instanceName, zone, err)
	}

	logrus.WithFields(logFields).Infof("Waiting for attach disk operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for attach disk operation %s failed", op.Name())
		return fmt.Errorf("waiting for attach disk to instance %s operation failed: %w", instanceName, err)
	}

	logrus.WithFields(logFields).Info("Disk attached successfully to instance.")
	return nil
}

// DetachDisk detaches a disk from a GCE instance.
func (ops *Clients) DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) error {
	logFields := logrus.Fields{
		"project":    projectID,
		"zone":       zone,
		"instance":   instanceName,
		"deviceName": deviceName,
	}
	logrus.WithFields(logFields).Info("Detaching disk from instance")

	req := &computepb.DetachDiskInstanceRequest{
		Project:    projectID,
		Zone:       zone,
		Instance:   instanceName,
		DeviceName: deviceName,
	}

	op, err := ops.Gce.DetachDisk(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate detach disk operation")
		return fmt.Errorf("failed to detach disk %s from instance %s in zone %s: %w", deviceName, instanceName, zone, err)
	}
	logrus.WithFields(logFields).Infof("Waiting for detach disk operation %s to complete...", op.Name())
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()
	err = op.Wait(opCtx)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for detach disk operation %s failed", op.Name())
		return fmt.Errorf("waiting for detach disk %s from instance %s operation failed: %w", deviceName, instanceName, err)
	}

	logrus.WithFields(logFields).Info("Disk detached successfully from instance.")
	return nil
}
