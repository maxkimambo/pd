package gcp

import (
	"context"
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
)

// InstanceOps handles GCE instance operations.
type InstanceOps struct {
	instancesClient *compute.InstancesClient // Using concrete type for specific instance operations
}

// NewInstanceOps creates a new InstanceOps.
// Note: You might want to pass GceClientInterface if it's extended with necessary methods,
// or pass the concrete *compute.InstancesClient from your Clients struct.
// For this example, we assume *compute.InstancesClient is passed directly.
func NewInstanceOps(client *compute.InstancesClient) *InstanceOps {
	return &InstanceOps{instancesClient: client}
}

// StartInstance starts a GCE instance.
func (ops *InstanceOps) StartInstance(ctx context.Context, projectID, zone, instanceName string) (*compute.Operation, error) {
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Starting instance")

	req := &computepb.StartInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ops.instancesClient.Start(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to start instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

// StopInstance stops a GCE instance.
func (ops *InstanceOps) StopInstance(ctx context.Context, projectID, zone, instanceName string) (*compute.Operation, error) {
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Stopping instance")

	req := &computepb.StopInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ops.instancesClient.Stop(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to stop instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

// ListInstancesInZone lists all instances in a specific zone.
func (ops *InstanceOps) ListInstancesInZone(ctx context.Context, projectID, zone string) *compute.InstanceIterator {
	logrus.WithFields(logrus.Fields{
		"project": projectID,
		"zone":    zone,
	}).Info("Listing instances in zone")

	req := &computepb.ListInstancesRequest{
		Project: projectID,
		Zone:    zone,
	}
	return ops.instancesClient.List(ctx, req)
}

// ListInstancesInProject lists all instances in a project (aggregated list).
func (ops *InstanceOps) ListInstancesInProject(ctx context.Context, projectID string) *compute.InstancesScopedListPairIterator {
	logrus.WithFields(logrus.Fields{
		"project": projectID,
	}).Info("Listing instances in project")

	req := &computepb.AggregatedListInstancesRequest{
		Project: projectID,
	}
	return ops.instancesClient.AggregatedList(ctx, req)
}

// GetInstance gets details of a specific GCE instance.
func (ops *InstanceOps) GetInstance(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error) {
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

	instance, err := ops.instancesClient.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, zone, err)
	}
	return instance, nil
}

// DeleteInstance deletes a GCE instance.
func (ops *InstanceOps) DeleteInstance(ctx context.Context, projectID, zone, instanceName string) (*compute.Operation, error) {
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
	}).Info("Deleting instance")

	req := &computepb.DeleteInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	}

	op, err := ops.instancesClient.Delete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

// AttachDisk attaches a disk to a GCE instance.
func (ops *InstanceOps) AttachDisk(ctx context.Context, projectID, zone, instanceName string, attachedDiskResource *computepb.AttachedDisk) (*compute.Operation, error) {
	logrus.WithFields(logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"instance": instanceName,
		"disk":     attachedDiskResource.GetSource(),
	}).Info("Attaching disk to instance")

	req := &computepb.AttachDiskInstanceRequest{
		Project:              projectID,
		Zone:                 zone,
		Instance:             instanceName,
		AttachedDiskResource: attachedDiskResource,
	}

	op, err := ops.instancesClient.AttachDisk(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to attach disk to instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

// DetachDisk detaches a disk from a GCE instance.
func (ops *InstanceOps) DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) (*compute.Operation, error) {
	logrus.WithFields(logrus.Fields{
		"project":    projectID,
		"zone":       zone,
		"instance":   instanceName,
		"deviceName": deviceName,
	}).Info("Detaching disk from instance")

	req := &computepb.DetachDiskInstanceRequest{
		Project:    projectID,
		Zone:       zone,
		Instance:   instanceName,
		DeviceName: deviceName,
	}

	op, err := ops.instancesClient.DetachDisk(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to detach disk %s from instance %s in zone %s: %w", deviceName, instanceName, zone, err)
	}
	return op, nil
}
