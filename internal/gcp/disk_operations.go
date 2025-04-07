package gcp

import (
	"context"
	"fmt"
	"strings"
	"time" // Import time package

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

// Define defaultOpTimeout here to match the one in disks.go
const defaultOpTimeout = 5 * time.Minute

func (c *Clients) ListDetachedDisks(
	ctx context.Context,
	projectID string,
	location string,
	labelFilter string,
) ([]*computepb.Disk, error) {
	logFields := logrus.Fields{
		"project": projectID,
		"zone":    location,
	}
	logrus.WithFields(logFields).Info("Listing detached disks...")

	req := &computepb.AggregatedListDisksRequest{
		Project: projectID,
	}

	it := c.Disks.AggregatedList(ctx, req)
	var disks []*computepb.Disk

	for {
		resp, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			logrus.WithFields(logFields).WithError(err).Error("Failed to list disks")
			return nil, fmt.Errorf("failed to list disks: %w", err)
		}

		for _, disk := range resp.Value.Disks {
			if *disk.Status == "READY" && *disk.Zone == location && len(disk.Users) == 0 {
				disks = append(disks, disk)
				logrus.WithFields(logFields).Infof("Found detached disk: %s", disk.Name)
			}
		}
	}

	logrus.WithFields(logFields).Infof("Found %d detached disk(s)", len(disks))
	return disks, nil
}

// CreateNewDiskFromSnapshot creates a new disk from a snapshot with the specified type and labels.
// This is a replacement for the problematic CreateDiskFromSnapshot function.
func (c *Clients) CreateNewDiskFromSnapshot(
	ctx context.Context,
	projectID string,
	zone string,
	newDiskName string,
	targetDiskType string,
	snapshotSource string,
	labels map[string]string,
) error {
	logFields := logrus.Fields{
		"project":        projectID,
		"zone":           zone,
		"newDisk":        newDiskName,
		"targetType":     targetDiskType,
		"snapshotSource": snapshotSource,
	}
	logrus.WithFields(logFields).Info("Initiating disk creation from snapshot...")

	// Construct the full snapshot source URL
	// Assuming snapshotSource is just the name, construct the global path
	if !strings.Contains(snapshotSource, "/") {
		snapshotSource = fmt.Sprintf("global/snapshots/%s", snapshotSource)
	}

	// Ensure target disk type URL is correct (e.g., zones/us-central1-a/diskTypes/pd-ssd)
	targetDiskTypeURL := fmt.Sprintf("zones/%s/diskTypes/%s", zone, targetDiskType)

	disk := &computepb.Disk{
		Name:           proto.String(newDiskName),
		Type:           proto.String(targetDiskTypeURL),
		SourceSnapshot: proto.String(snapshotSource),
		Labels:         labels,
		// SizeGb: // Size is usually inferred from snapshot, but can be specified if larger needed
	}

	req := &computepb.InsertDiskRequest{
		Project:      projectID,
		Zone:         zone,
		DiskResource: disk,
	}

	op, err := c.Disks.Insert(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate disk creation")
		return fmt.Errorf("failed to initiate creation for disk %s: %w", newDiskName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for disk creation operation %s to complete...", opName)

	// Wait for the operation to complete
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx) // Wait only returns error
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for disk creation operation %s failed", opName)
		return fmt.Errorf("waiting for disk %s creation failed: %w", newDiskName, err)
	}

	logrus.WithFields(logFields).Info("Disk created successfully from snapshot.")
	return nil
}

// UpdateDiskLabel adds or updates a label on a disk.
// This is a replacement for the problematic SetDiskLabel function.
func (c *Clients) UpdateDiskLabel(
	ctx context.Context,
	projectID string,
	zone string,
	diskName string,
	labelKey string,
	labelValue string,
) error {
	logFields := logrus.Fields{
		"project":  projectID,
		"zone":     zone,
		"disk":     diskName,
		"labelKey": labelKey,
		"labelVal": labelValue,
	}
	logrus.WithFields(logFields).Info("Setting disk label...")

	// 1. Get the current disk to obtain the label fingerprint
	// Get disk details directly instead of using c.GetDisk
	req := &computepb.GetDiskRequest{
		Project: projectID,
		Zone:    zone,
		Disk:    diskName,
	}

	currentDisk, err := c.Disks.Get(ctx, req)
	if err != nil {
		// Don't wrap error here as GetDisk already does
		return fmt.Errorf("failed to get current disk state before setting label: %w", err)
	}
	currentLabels := currentDisk.GetLabels()
	labelFingerprint := currentDisk.GetLabelFingerprint()

	if currentLabels == nil {
		currentLabels = make(map[string]string)
	}
	currentLabels[labelKey] = labelValue

	// 2. Set the labels using the fingerprint
	setLabelsReq := &computepb.SetLabelsDiskRequest{ // Use a different variable name
		Project:  projectID,
		Zone:     zone,
		Resource: diskName,
		ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
			Labels:           currentLabels,
			LabelFingerprint: proto.String(labelFingerprint),
		},
	}

	op, err := c.Disks.SetLabels(ctx, setLabelsReq) // Pass the correct request variable
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate set disk label operation")
		return fmt.Errorf("failed to initiate set label for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for set label operation %s to complete...", opName)

	// Wait for the operation to complete
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx) // Wait only returns error
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for set label operation %s failed", opName)
		return fmt.Errorf("waiting for disk %s set label failed: %w", diskName, err)
	}

	logrus.WithFields(logFields).Info("Disk label set successfully.")
	return nil
}

// DeleteDisk initiates disk deletion and waits for the operation to complete.
func (c *Clients) DeleteDisk(ctx context.Context, projectID, zone, diskName string) error {
	logFields := logrus.Fields{"project": projectID, "zone": zone, "disk": diskName}
	logrus.WithFields(logFields).Info("Initiating deletion of disk...")

	req := &computepb.DeleteDiskRequest{
		Project: projectID,
		Zone:    zone,
		Disk:    diskName,
	}

	op, err := c.Disks.Delete(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate disk deletion")
		return fmt.Errorf("failed to initiate deletion for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for disk deletion operation %s to complete...", opName)

	// Wait for the operation to complete
	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for disk deletion operation %s failed", opName)
		return fmt.Errorf("waiting for disk %s deletion failed: %w", diskName, err)
	}

	logrus.WithFields(logFields).Info("Disk deleted successfully.")
	return nil
}
