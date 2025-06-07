package gcp

import (
	"context"
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

type DiskClientInterface interface {
	AggregatedList(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gax.CallOption) *compute.DisksScopedListPairIterator
	Insert(ctx context.Context, req *computepb.InsertDiskRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Get(ctx context.Context, req *computepb.GetDiskRequest, opts ...gax.CallOption) (*computepb.Disk, error)
	SetLabels(ctx context.Context, req *computepb.SetLabelsDiskRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Delete(ctx context.Context, req *computepb.DeleteDiskRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Close() error
}

type SnapshotClientInterface interface {
	Insert(ctx context.Context, req *computepb.InsertSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Delete(ctx context.Context, req *computepb.DeleteSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error)
	List(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator
	Close() error
}

type ZoneClientInterface interface {
	Close() error
}

type RegionClientInterface interface {
	Close() error
}

type GceClientInterface interface {
	Start(ctx context.Context, req *computepb.StartInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Stop(ctx context.Context, req *computepb.StopInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	List(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) *compute.InstanceIterator
	AggregatedList(ctx context.Context, req *computepb.AggregatedListInstancesRequest, opts ...gax.CallOption) *compute.InstancesScopedListPairIterator
	Get(ctx context.Context, req *computepb.GetInstanceRequest, opts ...gax.CallOption) (*computepb.Instance, error)
	Delete(ctx context.Context, req *computepb.DeleteInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	AttachDisk(ctx context.Context, req *computepb.AttachDiskInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	DetachDisk(ctx context.Context, req *computepb.DetachDiskInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Close() error
}

// DiskOperationsInterface defines the high-level disk operations interface
type DiskOperationsInterface interface {
	GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error)
	ListDetachedDisks(ctx context.Context, projectID string, location string, labelFilter string) ([]*computepb.Disk, error)
	CreateNewDiskFromSnapshot(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, iops int64, throughput int64, storagePoolID string) error
	UpdateDiskLabel(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error
	DeleteDisk(ctx context.Context, projectID, zone, diskName string) error
}

type Clients struct {
	Disks          DiskClientInterface
	DiskClient     DiskOperationsInterface
	Snapshots      SnapshotClientInterface
	SnapshotClient SnapshotOperationsInterface
	Zones          ZoneClientInterface
	Regions        RegionClientInterface
	Gce            GceClientInterface
	InstanceClient InstanceOperationsInterface
}

func NewClients(ctx context.Context) (*Clients, error) {
	logrus.Debug("Initializing GCP Compute API client...")

	defaultOpts := getDefaultClientOptions()

	disksClient, err := compute.NewDisksRESTClient(ctx, defaultOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute Disks client: %w", err)
	}
	logrus.Debug("Disks client initialized.")

	snapshotsClient, err := compute.NewSnapshotsRESTClient(ctx, defaultOpts...)
	if err != nil {
		disksClient.Close()
		return nil, fmt.Errorf("failed to create compute Snapshots client: %w", err)
	}
	logrus.Debug("Snapshots client initialized.")

	zonesClient, err := compute.NewZonesRESTClient(ctx, defaultOpts...)
	if err != nil {
		disksClient.Close()
		snapshotsClient.Close()
		return nil, fmt.Errorf("failed to create compute Zones client: %w", err)
	}
	logrus.Debug("Zones client initialized.")

	regionsClient, err := compute.NewRegionsRESTClient(ctx, defaultOpts...)
	if err != nil {
		disksClient.Close()
		snapshotsClient.Close()
		zonesClient.Close()
		return nil, fmt.Errorf("failed to create compute Regions client: %w", err)
	}
	logrus.Debug("Regions client initialized.")

	gceClient, err := compute.NewInstancesRESTClient(ctx, defaultOpts...)
	if err != nil {
		disksClient.Close()
		snapshotsClient.Close()
		zonesClient.Close()
		regionsClient.Close()
		return nil, fmt.Errorf("failed to create compute Instances (GCE) client: %w", err)
	}
	logrus.Debug("GCE client initialized.")

	// Create the high-level clients
	diskClient := NewDiskClient(disksClient)
	snapshotClient := NewSnapshotClient(snapshotsClient)
	instanceClient := NewInstanceClient(gceClient)

	logrus.Info("Successfully initialized GCP Compute API clients.")
	return &Clients{
		Disks:          disksClient,
		DiskClient:     diskClient,
		Snapshots:      snapshotsClient,
		SnapshotClient: snapshotClient,
		Zones:          zonesClient,
		Regions:        regionsClient,
		Gce:            gceClient,
		InstanceClient: instanceClient,
	}, nil
}

func (c *Clients) Close() {
	logrus.Debug("Closing GCP Compute API clients...")
	if c.Disks != nil {
		c.Disks.Close()
	}
	if c.Snapshots != nil {
		c.Snapshots.Close()
	}
	if c.Zones != nil {
		c.Zones.Close()
	}
	if c.Regions != nil {
		c.Regions.Close()
	}
	if c.Gce != nil {
		c.Gce.Close()
	}
	logrus.Debug("GCP Compute API clients closed.")
}

func getDefaultClientOptions() []option.ClientOption {
	return nil
}
