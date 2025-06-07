package gcp

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

type SnapshotKmsParams struct {
	KmsKey      string
	KmsKeyRing  string
	KmsLocation string
	KmsProject  string
}

// SnapshotOperationsInterface defines the high-level snapshot operations interface
type SnapshotOperationsInterface interface {
	CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error
	DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error
	ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error)
}

// SnapshotClient wraps the GCP snapshot client and provides snapshot operation methods
type SnapshotClient struct {
	client SnapshotClientInterface
}

// NewSnapshotClient creates a new SnapshotClient with the provided SnapshotClientInterface
func NewSnapshotClient(client SnapshotClientInterface) *SnapshotClient {
	return &SnapshotClient{
		client: client,
	}
}

func (sc *SnapshotClient) CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error {
	logFields := logrus.Fields{
		"project":      projectID,
		"zone":         zone,
		"disk":         diskName,
		"snapshotName": snapshotName,
	}
	logrus.WithFields(logFields).Info("Initiating snapshot creation...")

	if labels == nil {
		labels = make(map[string]string)
	}
	labels["managed-by"] = "pd-migrate" 

	sourceDiskURL := fmt.Sprintf("projects/%s/zones/%s/disks/%s", projectID, zone, diskName)

	snapshotResource := &computepb.Snapshot{
		Name:       proto.String(snapshotName),
		Labels:     labels,
		SourceDisk: proto.String(sourceDiskURL), 
	}

	if kmsParams != nil && kmsParams.KmsKey != "" {
		logFields["kmsKey"] = kmsParams.KmsKey
		kmsProject := kmsParams.KmsProject
		if kmsProject == "" {
			kmsProject = projectID 
		}
		kmsKeyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
			kmsProject, kmsParams.KmsLocation, kmsParams.KmsKeyRing, kmsParams.KmsKey)
		snapshotResource.SnapshotEncryptionKey = &computepb.CustomerEncryptionKey{
			KmsKeyName: proto.String(kmsKeyName),
		}
		logrus.WithFields(logFields).Info("Applying KMS encryption to snapshot")
	}

	req := &computepb.InsertSnapshotRequest{
		Project: projectID,
		SnapshotResource: snapshotResource,
	}

	op, err := sc.client.Insert(ctx, req) 
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate snapshot creation")
		return fmt.Errorf("failed to initiate snapshot creation for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for snapshot creation operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout) 
	defer cancel()

	err = op.Wait(opCtx) 
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for snapshot creation operation %s failed", opName)
		return fmt.Errorf("waiting for snapshot %s creation failed: %w", snapshotName, err)
	}


	logrus.WithFields(logFields).Info("Snapshot created successfully.")
	return nil
}

func (sc *SnapshotClient) DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error {
	logFields := logrus.Fields{
		"project":      projectID,
		"snapshotName": snapshotName,
	}
	logrus.WithFields(logFields).Info("Initiating deletion of snapshot...")

	req := &computepb.DeleteSnapshotRequest{
		Project:  projectID,
		Snapshot: snapshotName,
	}

	op, err := sc.client.Delete(ctx, req)
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Error("Failed to initiate snapshot deletion")
		return fmt.Errorf("failed to initiate deletion for snapshot %s: %w", snapshotName, err)
	}

	opName := op.Name()
	logrus.WithFields(logFields).Infof("Waiting for snapshot deletion operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx) 
	if err != nil {
		logrus.WithFields(logFields).WithError(err).Errorf("Waiting for snapshot deletion operation %s failed", opName)
		return fmt.Errorf("waiting for snapshot %s deletion failed: %w", snapshotName, err)
	}


	logrus.WithFields(logFields).Info("Snapshot deleted successfully.")
	return nil
}

func (sc *SnapshotClient) ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
	snapshots := make([]*computepb.Snapshot, 0)
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

	it := sc.client.List(ctx, req)
	if it == nil {
		logrus.WithFields(logFields).Warn("List returned a nil iterator. Returning empty snapshot list.")
		return snapshots, nil
	}

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

