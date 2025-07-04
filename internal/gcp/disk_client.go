package gcp

import (
	"context"
	"fmt"
	"strings"

	"slices"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

func supportsIopsAndThroughput(diskType string) bool {
	supportedTypes := []string{
		"pd-extreme",
		"hyperdisk-balanced",
		"hyperdisk-extreme",
		"hyperdisk-ml",
	}
	return slices.Contains(supportedTypes, diskType)
}

type DiskClientInterface interface {
	GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error)
	ListDetachedDisks(ctx context.Context, projectID string, location string, labelFilter string) ([]*computepb.Disk, error)
	CreateNewDiskFromSnapshot(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, size int64, iops int64, throughput int64, storagePoolID string) error
	UpdateDiskLabel(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error
	DeleteDisk(ctx context.Context, projectID, zone, diskName string) error
	Close() error
}

type DiskClient struct {
	client *compute.DisksClient
}

func NewDiskClient(client *compute.DisksClient) *DiskClient {
	return &DiskClient{
		client: client,
	}
}

func (dc *DiskClient) GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
	logFields := map[string]interface{}{
		"project": projectID,
		"zone":    zone,
		"disk":    diskName,
	}
	logger.WithFieldsMap(logFields).Info("Retrieving disk information...")

	req := &computepb.GetDiskRequest{
		Project: projectID,
		Zone:    zone,
		Disk:    diskName,
	}

	disk, err := dc.client.Get(ctx, req)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Error("Failed to retrieve disk")
		return nil, fmt.Errorf("failed to get disk %s in zone %s: %w", diskName, zone, err)
	}

	logger.WithFieldsMap(logFields).Infof("Retrieved disk: %s", *disk.Name)
	return disk, nil
}

func (dc *DiskClient) ListDetachedDisks(
	ctx context.Context,
	projectID string,
	location string,
	labelFilter string,
) ([]*computepb.Disk, error) {
	logFields := map[string]interface{}{
		"project": projectID,
		"zone":    location,
	}
	logger.WithFieldsMap(logFields).Info("Listing detached disks...")

	req := &computepb.AggregatedListDisksRequest{
		Project: projectID,
	}

	it := dc.client.AggregatedList(ctx, req)
	disks := make([]*computepb.Disk, 0)
	if it == nil {
		logger.WithFieldsMap(logFields).Warn("AggregatedList returned a nil iterator. Returning empty disk list.")
		return disks, nil
	}

	for {
		resp, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			logger.WithFieldsMap(logFields).WithError(err).Error("Failed to list disks")
			return nil, fmt.Errorf("failed to list disks: %w", err)
		}

		for _, disk := range resp.Value.Disks {
			zone := utils.ExtractZoneName(disk.GetZone())
			if *disk.Status == "READY" && zone == location && len(disk.Users) == 0 {
				disks = append(disks, disk)
				logger.WithFieldsMap(logFields).Infof("Found detached disk: %s", *disk.Name)
			}
		}
	}

	logger.WithFieldsMap(logFields).Infof("Found %d detached disk(s)", len(disks))
	return disks, nil
}

func (dc *DiskClient) CreateNewDiskFromSnapshot(
	ctx context.Context,
	projectID string,
	zone string,
	newDiskName string,
	targetDiskType string,
	snapshotSource string,
	labels map[string]string,
	size int64,
	iops int64,
	throughput int64,
	storagePoolID string,
) error {
	logFields := map[string]interface{}{
		"project":        projectID,
		"zone":           zone,
		"newDisk":        newDiskName,
		"targetType":     targetDiskType,
		"snapshotSource": snapshotSource,
	}
	logger.WithFieldsMap(logFields).Info("Initiating disk creation from snapshot...")

	if !strings.Contains(snapshotSource, "/") {
		snapshotSource = fmt.Sprintf("global/snapshots/%s", snapshotSource)
	}

	targetDiskTypeURL := fmt.Sprintf("zones/%s/diskTypes/%s", zone, targetDiskType)
	var disk *computepb.Disk
	if !supportsIopsAndThroughput(targetDiskType) {
		disk = &computepb.Disk{
			Name:           proto.String(newDiskName),
			Type:           proto.String(targetDiskTypeURL),
			SourceSnapshot: proto.String(snapshotSource),
			Labels:         labels,
			SizeGb:         proto.Int64(size),
		}
	} else {
		disk = &computepb.Disk{
			Name:                  proto.String(newDiskName),
			Type:                  proto.String(targetDiskTypeURL),
			SourceSnapshot:        proto.String(snapshotSource),
			Labels:                labels,
			ProvisionedIops:       proto.Int64(iops),
			ProvisionedThroughput: proto.Int64(throughput),
			SizeGb:                proto.Int64(size),
		}
	}
	// If storagePoolID is provided, set it on the disk
	// This is only applicable for certain disk types that support storage pools
	// such as Hyperdisk.
	// If the disk type does not support storage pools, this field will be ignored.
	if storagePoolID != "" {
		disk.StoragePool = proto.String(storagePoolID)
	}

	req := &computepb.InsertDiskRequest{
		Project:      projectID,
		Zone:         zone,
		DiskResource: disk,
	}

	op, err := dc.client.Insert(ctx, req)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Error("Failed to initiate disk creation")
		return fmt.Errorf("failed to initiate creation for disk %s: %w", newDiskName, err)
	}

	opName := op.Name()
	logger.WithFieldsMap(logFields).Infof("Waiting for disk creation operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Errorf("Waiting for disk creation operation %s failed", opName)
		return fmt.Errorf("waiting for disk %s creation failed: %w", newDiskName, err)
	}

	logger.WithFieldsMap(logFields).Info("Disk created successfully from snapshot.")
	return nil
}

func (dc *DiskClient) UpdateDiskLabel(
	ctx context.Context,
	projectID string,
	zone string,
	diskName string,
	labelKey string,
	labelValue string,
) error {
	logFields := map[string]interface{}{
		"project":  projectID,
		"zone":     zone,
		"disk":     diskName,
		"labelKey": labelKey,
		"labelVal": labelValue,
	}
	logger.WithFieldsMap(logFields).Info("Setting disk label...")

	req := &computepb.GetDiskRequest{
		Project: projectID,
		Zone:    zone,
		Disk:    diskName,
	}

	currentDisk, err := dc.client.Get(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get current disk state before setting label: %w", err)
	}
	currentLabels := currentDisk.GetLabels()
	labelFingerprint := currentDisk.GetLabelFingerprint()

	if currentLabels == nil {
		currentLabels = make(map[string]string)
	}
	currentLabels[labelKey] = labelValue

	setLabelsReq := &computepb.SetLabelsDiskRequest{
		Project:  projectID,
		Zone:     zone,
		Resource: diskName,
		ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
			Labels:           currentLabels,
			LabelFingerprint: proto.String(labelFingerprint),
		},
	}

	op, err := dc.client.SetLabels(ctx, setLabelsReq)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Error("Failed to initiate set disk label operation")
		return fmt.Errorf("failed to initiate set label for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logger.WithFieldsMap(logFields).Infof("Waiting for set label operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Errorf("Waiting for set label operation %s failed", opName)
		return fmt.Errorf("waiting for disk %s set label failed: %w", diskName, err)
	}

	logger.WithFieldsMap(logFields).Info("Disk label set successfully.")
	return nil
}

func (dc *DiskClient) DeleteDisk(ctx context.Context, projectID, zone, diskName string) error {
	logFields := map[string]interface{}{"project": projectID, "zone": zone, "disk": diskName}
	logger.WithFieldsMap(logFields).Info("Initiating deletion of disk...")

	req := &computepb.DeleteDiskRequest{
		Project: projectID,
		Zone:    zone,
		Disk:    diskName,
	}

	op, err := dc.client.Delete(ctx, req)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Error("Failed to initiate disk deletion")
		return fmt.Errorf("failed to initiate deletion for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logger.WithFieldsMap(logFields).Infof("Waiting for disk deletion operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx)
	if err != nil {
		logger.WithFieldsMap(logFields).WithError(err).Errorf("Waiting for disk deletion operation %s failed", opName)
		return fmt.Errorf("waiting for disk %s deletion failed: %w", diskName, err)
	}

	logger.WithFieldsMap(logFields).Info("Disk deleted successfully.")
	return nil
}

func (dc *DiskClient) Close() error {
	return dc.client.Close()
}
