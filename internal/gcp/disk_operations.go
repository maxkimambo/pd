package gcp

import (
	"context"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

func (c *Clients) GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
	return c.DiskClient.GetDisk(ctx, projectID, zone, diskName)
}

func (c *Clients) ListDetachedDisks(
	ctx context.Context,
	projectID string,
	location string,
	labelFilter string,
) ([]*computepb.Disk, error) {
	return c.DiskClient.ListDetachedDisks(ctx, projectID, location, labelFilter)
}

func (c *Clients) CreateNewDiskFromSnapshot(
	ctx context.Context,
	projectID string,
	zone string,
	newDiskName string,
	targetDiskType string,
	snapshotSource string,
	labels map[string]string,
	iops int64,
	throughput int64,
	storagePoolID string,
) error {
	return c.DiskClient.CreateNewDiskFromSnapshot(ctx, projectID, zone, newDiskName, targetDiskType, snapshotSource, labels, iops, throughput, storagePoolID)
}

func (c *Clients) UpdateDiskLabel(
	ctx context.Context,
	projectID string,
	zone string,
	diskName string,
	labelKey string,
	labelValue string,
) error {
	return c.DiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)
}

func (c *Clients) DeleteDisk(ctx context.Context, projectID, zone, diskName string) error {
	return c.DiskClient.DeleteDisk(ctx, projectID, zone, diskName)
}
