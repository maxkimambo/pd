package gcp

import (
	"context"
	"fmt"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/google/uuid"
	"github.com/maxkimambo/pd/internal/logger"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

const (
	MANAGED_BY_KEY   = "managed-by"
	MANAGED_BY_VALUE = "pd-migrate"
	
	// Enhanced labeling constants for migration session tracking
	SESSION_ID_KEY    = "migration-session-id"
	TASK_ID_KEY       = "migration-task-id" 
	CREATED_AT_KEY    = "created-at"
	SOURCE_DISK_KEY   = "source-disk"
	CLEANUP_AFTER_KEY = "cleanup-after"
)

type SnapshotKmsParams struct {
	KmsKey      string
	KmsKeyRing  string
	KmsLocation string
	KmsProject  string
}

// SnapshotMetadata holds enhanced metadata for migration snapshots
type SnapshotMetadata struct {
	SessionID     string
	TaskID        string
	CreatedAt     time.Time
	SourceDisk    string
	CleanupAfter  time.Time
	Labels        map[string]string
}

// NewSnapshotMetadata creates enhanced metadata for migration snapshots
func NewSnapshotMetadata(sessionID, taskID, sourceDisk string, cleanupAfterDuration time.Duration) *SnapshotMetadata {
	now := time.Now()
	cleanupAfter := now.Add(cleanupAfterDuration)
	
	return &SnapshotMetadata{
		SessionID:    sessionID,
		TaskID:       taskID,
		CreatedAt:    now,
		SourceDisk:   sourceDisk,
		CleanupAfter: cleanupAfter,
		Labels:       make(map[string]string),
	}
}

// ToLabels converts metadata to GCP labels format
func (sm *SnapshotMetadata) ToLabels() map[string]string {
	labels := make(map[string]string)
	
	// Copy custom labels first
	for k, v := range sm.Labels {
		labels[k] = v
	}
	
	// Add standard migration labels
	labels[MANAGED_BY_KEY] = MANAGED_BY_VALUE
	labels[SESSION_ID_KEY] = sm.SessionID
	labels[TASK_ID_KEY] = sm.TaskID
	labels[CREATED_AT_KEY] = sm.CreatedAt.Format("2025-01-02T15-04-05Z")
	labels[SOURCE_DISK_KEY] = sm.SourceDisk
	labels[CLEANUP_AFTER_KEY] = sm.CleanupAfter.Format("2025-01-02T15-04-05Z")

	return labels
}

// GenerateSessionID creates a new session UUID for migration tracking
func GenerateSessionID() string {
	return uuid.New().String()
}

// SnapshotClientInterface defines the high-level snapshot operations interface
type SnapshotClientInterface interface {
	// CreateSnapshot creates a new snapshot from the specified disk
	CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error
	// CreateSnapshotWithMetadata creates a snapshot with enhanced migration metadata
	CreateSnapshotWithMetadata(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, metadata *SnapshotMetadata) error
	// GetSnapshot retrieves a snapshot by name
	GetSnapshot(ctx context.Context, projectID, snapshotName string) (*computepb.Snapshot, error)
	// DeleteSnapshot deletes the specified snapshot
	DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error
	// ListSnapshotsByLabel lists snapshots that match the specified label key-value pair
	ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error)
	// ListSnapshotsBySessionID lists all snapshots for a specific migration session
	ListSnapshotsBySessionID(ctx context.Context, projectID, sessionID string) ([]*computepb.Snapshot, error)
	// ListExpiredSnapshots lists snapshots that are past their cleanup time
	ListExpiredSnapshots(ctx context.Context, projectID string) ([]*computepb.Snapshot, error)
	// DeleteSnapshotsBySessionID deletes all snapshots for a specific migration session
	DeleteSnapshotsBySessionID(ctx context.Context, projectID, sessionID string) error
	// Close closes the underlying client connections
	Close() error
}

// SnapshotClient wraps the GCP snapshot client and provides snapshot operation methods
type SnapshotClient struct {
	client *compute.SnapshotsClient
}

// NewSnapshotClient creates a new SnapshotClient with the provided *compute.SnapshotsClient
func NewSnapshotClient(client *compute.SnapshotsClient) *SnapshotClient {
	return &SnapshotClient{
		client: client,
	}
}

func (sc *SnapshotClient) CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error {
	logFields := map[string]interface{}{
		"project":      projectID,
		"zone":         zone,
		"disk":         diskName,
		"snapshotName": snapshotName,
	}
	logger.Op.WithFields(logFields).Info("Initiating snapshot creation...")

	if labels == nil {
		labels = make(map[string]string)
	}
	labels[MANAGED_BY_KEY] = MANAGED_BY_VALUE

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
		logger.Op.WithFields(logFields).Info("Applying KMS encryption to snapshot")
	}

	req := &computepb.InsertSnapshotRequest{
		Project:          projectID,
		SnapshotResource: snapshotResource,
	}

	op, err := sc.client.Insert(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to initiate snapshot creation")
		return fmt.Errorf("failed to initiate snapshot creation for disk %s: %w", diskName, err)
	}

	opName := op.Name()
	logger.Op.WithFields(logFields).Infof("Waiting for snapshot creation operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for snapshot creation operation %s failed", opName)
		return fmt.Errorf("waiting for snapshot %s creation failed: %w", snapshotName, err)
	}

	logger.Op.WithFields(logFields).Info("Snapshot created successfully.")
	return nil
}

// CreateSnapshotWithMetadata creates a snapshot with enhanced migration metadata
func (sc *SnapshotClient) CreateSnapshotWithMetadata(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, metadata *SnapshotMetadata) error {
	logFields := map[string]interface{}{
		"project":      projectID,
		"zone":         zone,
		"disk":         diskName,
		"snapshotName": snapshotName,
		"sessionID":    metadata.SessionID,
		"taskID":       metadata.TaskID,
	}
	logger.Op.WithFields(logFields).Info("Initiating snapshot creation with enhanced metadata...")

	// Merge metadata labels with any existing disk labels
	diskLabels := make(map[string]string)
	
	// Get disk to inherit its labels
	disk, err := sc.getDiskForSnapshot(ctx, projectID, zone, diskName)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Warn("Failed to get disk labels, proceeding without them")
	} else if disk.GetLabels() != nil {
		for k, v := range disk.GetLabels() {
			diskLabels[k] = v
		}
	}
	
	// Add disk labels to metadata
	for k, v := range diskLabels {
		metadata.Labels[k] = v
	}
	
	// Convert metadata to labels
	allLabels := metadata.ToLabels()
	
	return sc.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, kmsParams, allLabels)
}

// getDiskForSnapshot is a helper to get disk information for snapshot creation
func (sc *SnapshotClient) getDiskForSnapshot(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
	// We need a disk client to get disk info. For now, return nil and let caller handle it.
	// This will be enhanced when we integrate with the migrator package.
	return nil, fmt.Errorf("disk client not available in snapshot client")
}

// GetSnapshot retrieves a snapshot by name
func (sc *SnapshotClient) GetSnapshot(ctx context.Context, projectID, snapshotName string) (*computepb.Snapshot, error) {
	logFields := map[string]interface{}{
		"project":      projectID,
		"snapshotName": snapshotName,
	}
	logger.Op.WithFields(logFields).Debug("Getting snapshot...")

	req := &computepb.GetSnapshotRequest{
		Project:  projectID,
		Snapshot: snapshotName,
	}

	snapshot, err := sc.client.Get(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to get snapshot")
		return nil, fmt.Errorf("failed to get snapshot %s: %w", snapshotName, err)
	}

	logger.Op.WithFields(logFields).Debug("Snapshot retrieved successfully.")
	return snapshot, nil
}

func (sc *SnapshotClient) DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error {
	logFields := map[string]interface{}{
		"project":      projectID,
		"snapshotName": snapshotName,
	}
	logger.Op.WithFields(logFields).Info("Initiating deletion of snapshot...")

	req := &computepb.DeleteSnapshotRequest{
		Project:  projectID,
		Snapshot: snapshotName,
	}

	op, err := sc.client.Delete(ctx, req)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to initiate snapshot deletion")
		return fmt.Errorf("failed to initiate deletion for snapshot %s: %w", snapshotName, err)
	}

	opName := op.Name()
	logger.Op.WithFields(logFields).Infof("Waiting for snapshot deletion operation %s to complete...", opName)

	opCtx, cancel := context.WithTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err = op.Wait(opCtx)
	if err != nil {
		logger.Op.WithFields(logFields).WithError(err).Errorf("Waiting for snapshot deletion operation %s failed", opName)
		return fmt.Errorf("waiting for snapshot %s deletion failed: %w", snapshotName, err)
	}

	logger.Op.WithFields(logFields).Info("Snapshot deleted successfully.")
	return nil
}

func (sc *SnapshotClient) ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
	snapshots := make([]*computepb.Snapshot, 0)
	filter := fmt.Sprintf("labels.%s = %s", labelKey, labelValue)
	logFields := map[string]interface{}{
		"project": projectID,
		"filter":  filter,
	}
	logger.Op.WithFields(logFields).Info("Listing snapshots by label...")

	req := &computepb.ListSnapshotsRequest{
		Project: projectID,
		Filter:  proto.String(filter),
	}

	it := sc.client.List(ctx, req)
	if it == nil {
		logger.Op.WithFields(logFields).Warn("List returned a nil iterator. Returning empty snapshot list.")
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

	logger.Op.WithFields(logFields).Infof("Found %d snapshots matching label.", len(snapshots))
	return snapshots, nil
}

// ListSnapshotsBySessionID lists all snapshots for a specific migration session
func (sc *SnapshotClient) ListSnapshotsBySessionID(ctx context.Context, projectID, sessionID string) ([]*computepb.Snapshot, error) {
	logFields := map[string]interface{}{
		"project":   projectID,
		"sessionID": sessionID,
	}
	logger.Op.WithFields(logFields).Info("Listing snapshots by session ID...")

	return sc.ListSnapshotsByLabel(ctx, projectID, SESSION_ID_KEY, sessionID)
}

// ListExpiredSnapshots lists snapshots that are past their cleanup time
func (sc *SnapshotClient) ListExpiredSnapshots(ctx context.Context, projectID string) ([]*computepb.Snapshot, error) {
	logFields := map[string]interface{}{
		"project": projectID,
	}
	logger.Op.WithFields(logFields).Info("Listing expired snapshots...")

	// First get all migration snapshots
	allSnapshots, err := sc.ListSnapshotsByLabel(ctx, projectID, MANAGED_BY_KEY, MANAGED_BY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("failed to list migration snapshots: %w", err)
	}

	expiredSnapshots := make([]*computepb.Snapshot, 0)
	now := time.Now()

	for _, snapshot := range allSnapshots {
		if snapshot.GetLabels() == nil {
			continue
		}
		
		cleanupAfterStr, exists := snapshot.GetLabels()[CLEANUP_AFTER_KEY]
		if !exists {
			continue
		}
		
		cleanupAfter, err := time.Parse("2025-01-02T15-04-05Z", cleanupAfterStr)
		if err != nil {
			logger.Op.WithFields(logFields).WithError(err).Warnf("Failed to parse cleanup time for snapshot %s", snapshot.GetName())
			continue
		}
		
		if now.After(cleanupAfter) {
			expiredSnapshots = append(expiredSnapshots, snapshot)
		}
	}

	logger.Op.WithFields(logFields).Infof("Found %d expired snapshots out of %d total migration snapshots.", len(expiredSnapshots), len(allSnapshots))
	return expiredSnapshots, nil
}

// DeleteSnapshotsBySessionID deletes all snapshots for a specific migration session
func (sc *SnapshotClient) DeleteSnapshotsBySessionID(ctx context.Context, projectID, sessionID string) error {
	logFields := map[string]interface{}{
		"project":   projectID,
		"sessionID": sessionID,
	}
	logger.Op.WithFields(logFields).Info("Deleting all snapshots for session...")

	snapshots, err := sc.ListSnapshotsBySessionID(ctx, projectID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to list snapshots for session: %w", err)
	}

	if len(snapshots) == 0 {
		logger.Op.WithFields(logFields).Info("No snapshots found for session")
		return nil
	}

	logger.Op.WithFields(logFields).Infof("Found %d snapshots to delete for session", len(snapshots))
	
	deletedCount := 0
	failedCount := 0
	
	for _, snapshot := range snapshots {
		snapshotName := snapshot.GetName()
		snapshotLogFields := map[string]interface{}{
			"project":      projectID,
			"sessionID":    sessionID,
			"snapshotName": snapshotName,
		}
		
		logger.Op.WithFields(snapshotLogFields).Info("Deleting session snapshot...")
		
		err := sc.DeleteSnapshot(ctx, projectID, snapshotName)
		if err != nil {
			logger.Op.WithFields(snapshotLogFields).WithError(err).Error("Failed to delete session snapshot")
			failedCount++
		} else {
			logger.Op.WithFields(snapshotLogFields).Info("Session snapshot deleted successfully")
			deletedCount++
		}
	}

	resultFields := map[string]interface{}{
		"project":     projectID,
		"sessionID":   sessionID,
		"deleted":     deletedCount,
		"failed":      failedCount,
		"total":       len(snapshots),
	}
	
	if failedCount > 0 {
		logger.Op.WithFields(resultFields).Warnf("Session cleanup completed with %d failures out of %d snapshots", failedCount, len(snapshots))
		return fmt.Errorf("failed to delete %d out of %d snapshots for session %s", failedCount, len(snapshots), sessionID)
	}
	
	logger.Op.WithFields(resultFields).Info("All session snapshots deleted successfully")
	return nil
}

func (sc *SnapshotClient) Close() error {
	return sc.client.Close()
}
