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

type Clients struct {
	Disks     DiskClientInterface
	Snapshots SnapshotClientInterface
	Zones     ZoneClientInterface
	Regions   RegionClientInterface
}

func NewClients(ctx context.Context) (*Clients, error) {
	logrus.Debug("Initializing GCP Compute API clients...")

	disksClient, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute Disks client: %w", err)
	}
	logrus.Debug("Disks client initialized.")

	snapshotsClient, err := compute.NewSnapshotsRESTClient(ctx)
	if err != nil {
		disksClient.Close()
		return nil, fmt.Errorf("failed to create compute Snapshots client: %w", err)
	}
	logrus.Debug("Snapshots client initialized.")

	zonesClient, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		disksClient.Close()
		snapshotsClient.Close()
		return nil, fmt.Errorf("failed to create compute Zones client: %w", err)
	}
	logrus.Debug("Zones client initialized.")

	regionsClient, err := compute.NewRegionsRESTClient(ctx)
	if err != nil {
		disksClient.Close()
		snapshotsClient.Close()
		zonesClient.Close()
		return nil, fmt.Errorf("failed to create compute Regions client: %w", err)
	}
	logrus.Debug("Regions client initialized.")

	logrus.Info("Successfully initialized GCP Compute API clients.")
	return &Clients{
		Disks:     disksClient,
		Snapshots: snapshotsClient,
		Zones:     zonesClient,
		Regions:   regionsClient,
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
	logrus.Debug("GCP Compute API clients closed.")
}

func getDefaultClientOptions() []option.ClientOption {
	return nil
}
