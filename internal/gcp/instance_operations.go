package gcp

import (
	"context"
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

func (ops *Clients) StartInstance(ctx context.Context, projectID, zone, instanceName string) (*compute.Operation, error) {
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

	op, err := ops.Gce.Start(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to start instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

func (ops *Clients) StopInstance(ctx context.Context, projectID, zone, instanceName string) (*compute.Operation, error) {
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

	op, err := ops.Gce.Stop(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to stop instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

func (ops *Clients) ListInstancesInZone(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error) {
	logrus.WithFields(logrus.Fields{
		"project": projectID,
		"zone":    zone,
	}).Info("Listing instances in zone")

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
		"project": projectID,
		"zone":    zone,
		"count":   len(instances),
	}).Info("Successfully listed instances in zone.")
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

func (ops *Clients) DeleteInstance(ctx context.Context, projectID, zone, instanceName string) (*compute.Operation, error) {
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

	op, err := ops.Gce.Delete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

// AttachDisk attaches a disk to a GCE instance.
func (ops *Clients) AttachDisk(ctx context.Context, projectID, zone, instanceName string, attachedDiskResource *computepb.AttachedDisk) (*compute.Operation, error) {
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

	op, err := ops.Gce.AttachDisk(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to attach disk to instance %s in zone %s: %w", instanceName, zone, err)
	}
	return op, nil
}

// DetachDisk detaches a disk from a GCE instance.
func (ops *Clients) DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) (*compute.Operation, error) {
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

	op, err := ops.Gce.DetachDisk(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to detach disk %s from instance %s in zone %s: %w", deviceName, instanceName, zone, err)
	}
	return op, nil
}
