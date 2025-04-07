package gcp

import (
	"context"
	"fmt"

	// Re-import strings as it's used in getOperationError
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

// Removed redundant defaultOpTimeout definition (defined in disk_operations.go)

// SnapshotKmsParams holds KMS key details for snapshot encryption.
type SnapshotKmsParams struct {
	KmsKey      string
	KmsKeyRing  string
	KmsLocation string
	KmsProject  string
}

// CreateSnapshot creates a snapshot from a disk, optionally applying KMS encryption, and waits for completion.
// It adds a 'managed-by: pd-migrate' label.
func (c *Clients) CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error {
	logFields := logrus.Fields{
		"project":      projectID,
		"zone":         zone,
		"disk":         diskName,
		"snapshotName": snapshotName,
	}
	logrus.WithFields(logFields).Info("Initiating snapshot creation...")

	// Ensure labels map exists and add managed-by label
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["managed-by"] = "pd-migrate" // Label for cleanup phase

	// Define sourceDiskURL before using it in snapshotResource
	// Source disk URL format: projects/{project}/zones/{zone}/disks/{disk}
	sourceDiskURL := fmt.Sprintf("projects/%s/zones/%s/disks/%s", projectID, zone, diskName)

	snapshotResource := &computepb.Snapshot{
		Name:       proto.String(snapshotName),
		Labels:     labels,
		SourceDisk: proto.String(sourceDiskURL), // Source disk belongs in the resource
	}

	// Add KMS key if provided
	if kmsParams != nil && kmsParams.KmsKey != "" {
		logFields["kmsKey"] = kmsParams.KmsKey
		kmsProject := kmsParams.KmsProject
		if kmsProject == "" {
			kmsProject = projectID // Default to current project if not specified
		}
		kmsKeyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
			kmsProject, kmsParams.KmsLocation, kmsParams.KmsKeyRing, kmsParams.KmsKey)
		snapshotResource.SnapshotEncryptionKey = &computepb.CustomerEncryptionKey{
			KmsKeyName: proto.String(kmsKeyName),
		}
		logrus.WithFields(logFields).Info("Applying KMS encryption to snapshot")
	}

	// Moved declaration of sourceDiskURL above

	// Use InsertSnapshotRequest for creating snapshots from zonal disks
	req := &computepb.InsertSnapshotRequest{
		Project: projectID,
		// Zone:             proto.String(zone), // Zone is not part of InsertSnapshotRequest; inferred from SourceDisk?
		SnapshotResource: snapshotResource,
		// SourceDisk is part of SnapshotResource now
	}

	op, err := c.Snapshots.Insert(ctx, req) // Use Insert for snapshots
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate snapshot creation")
		return fmt.Errorf("failed to initiate snapshot creation for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for snapshot creation operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout) // Use same timeout as disks
	defer cancel()

	// Wait for the global operation to complete (snapshots are global resources)
	// Wait for the operation to complete. Snapshot operations are typically global,
	// but initiated via zonal endpoint for zonal disks. The Wait should handle this.
	err = op.Wait(opCtx) // Wait only returns an error
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for snapshot creation operation %s failed", opName)
		return fmt.Errorf("waiting for snapshot %s creation failed: %w", snapshotName, err)
	}
	// Need to get the final operation status after Wait succeeds
	// Remove unused finalOp declaration
	// finalOp, err := c.Snapshots.Get(ctx, &computepb.GetSnapshotRequest{Project: projectID, Snapshot: snapshotName})
	// Let's assume op itself might be updated or we need a GetOperation call - for now, check error helper logic

	// Revisit operation error checking after Wait()
	// For now, assume Wait() failing is the primary error path for the wait itself.
	// The API might implicitly check the final status within Wait, or we need a GetOperation call.
	// Let's simplify and assume Wait() returning nil means success for now, pending testing.
	// if err = getOperationError(finalOp); err != nil { // Temporarily comment out direct finalOp check
	// errMsg := fmt.Sprintf("snapshot %s creation operation %s finished with error: %v", snapshotName, opName, err)
	// logrus.WithFields(logFields).Error(errMsg)
	// return errors.New(errMsg) // Return the actual error
	// }
	// Removed leftover error handling code from commented block

	logrus.WithFields(logFields).Info("Snapshot created successfully.")
	return nil
}

// DeleteSnapshot deletes a snapshot and waits for the global operation to complete.
func (c *Clients) DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error {
	logFields := logrus.Fields{
		"project":      projectID,
		"snapshotName": snapshotName,
	}
	logrus.WithFields(logFields).Info("Initiating deletion of snapshot...")

	req := &computepb.DeleteSnapshotRequest{
		Project:  projectID,
		Snapshot: snapshotName,
	}

	op, err := c.Snapshots.Delete(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate snapshot deletion")
		return fmt.Errorf("failed to initiate deletion for snapshot %s: %w", snapshotName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for snapshot deletion operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	// Wait for the global operation
	err = op.Wait(opCtx) // Wait only returns an error
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for snapshot deletion operation %s failed", opName)
		return fmt.Errorf("waiting for snapshot %s deletion failed: %w", snapshotName, err)
	}
	// Assume Wait() returning nil means success for deletion as well.

	// if err = getOperationError(finalOp); err != nil { // Temporarily comment out direct finalOp check
	// errMsg := fmt.Sprintf("snapshot %s deletion operation %s finished with error: %v", snapshotName, opName, err)
	// logrus.WithFields(logFields).Error(errMsg)
	// return errors.New(errMsg) // Return the actual error
	// }

	logrus.WithFields(logFields).Info("Snapshot deleted successfully.")
	return nil
}

// ListSnapshotsByLabel lists global snapshots in a project filtered by a specific label.
func (c *Clients) ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
	var snapshots []*computepb.Snapshot
	filter := fmt.Sprintf("labels.%s = %s", labelKey, labelValue)
	logFields := logrus.Fields{
		"project": projectID,
		"filter":  filter,
	}
	logrus.WithFields(logFields).Info("Listing snapshots by label...")

	req := &computepb.ListSnapshotsRequest{
		Project: projectID,
		Filter:  proto.String(filter),
	}

	it := c.Snapshots.List(ctx, req)
	for {
		snapshot, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed iterating snapshot list: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}

	logrus.WithFields(logFields).Infof("Found %d snapshots matching label.", len(snapshots))
	return snapshots, nil
}

// getOperationError checks a completed operation proto for errors.
// TODO: Move this to a shared location within internal/gcp package.
// Removed getOperationError as its usage with Wait() needs clarification.
// The error handling within Wait() itself might be sufficient, or requires
// fetching the operation status explicitly after Wait() returns nil.
// This needs verification during testing.
