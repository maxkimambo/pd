package gcp

import (
	"context"
	"fmt"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/maxkimambo/pd/internal/logger"
	"google.golang.org/api/option"
)

const defaultOpTimeout = 10 * time.Minute

type ZoneClientInterface interface {
	Close() error
}

type RegionClientInterface interface {
	Close() error
}

type Clients struct {
	DiskClient     DiskClientInterface
	SnapshotClient SnapshotClientInterface
	ComputeClient  ComputeClientInterface
	Zones          ZoneClientInterface
	Regions        RegionClientInterface
}

func NewClients(ctx context.Context) (*Clients, error) {
	logger.Op.Debug("Initializing GCP Compute API client...")

	defaultOpts := getDefaultClientOptions()

	disksClient, err := compute.NewDisksRESTClient(ctx, defaultOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute Disks client: %w", err)
	}
	logger.Op.Debug("Disks client initialized.")

	snapshotsClient, err := compute.NewSnapshotsRESTClient(ctx, defaultOpts...)
	if err != nil {
		_ = disksClient.Close()
		return nil, fmt.Errorf("failed to create compute Snapshots client: %w", err)
	}
	logger.Op.Debug("Snapshots client initialized.")

	zonesClient, err := compute.NewZonesRESTClient(ctx, defaultOpts...)
	if err != nil {
		_ = disksClient.Close()
		_ = snapshotsClient.Close()
		return nil, fmt.Errorf("failed to create compute Zones client: %w", err)
	}
	logger.Op.Debug("Zones client initialized.")

	regionsClient, err := compute.NewRegionsRESTClient(ctx, defaultOpts...)
	if err != nil {
		_ = disksClient.Close()
		_ = snapshotsClient.Close()
		_ = zonesClient.Close()
		return nil, fmt.Errorf("failed to create compute Regions client: %w", err)
	}
	logger.Op.Debug("Regions client initialized.")

	gceClient, err := compute.NewInstancesRESTClient(ctx, defaultOpts...)
	if err != nil {
		_ = disksClient.Close()
		_ = snapshotsClient.Close()
		_ = zonesClient.Close()
		_ = regionsClient.Close()
		return nil, fmt.Errorf("failed to create compute Instances (GCE) client: %w", err)
	}
	logger.Op.Debug("GCE client initialized.")

	// Create the high-level clients
	diskClient := NewDiskClient(disksClient)
	snapshotClient := NewSnapshotClient(snapshotsClient)
	computeClient := NewComputeClient(gceClient, disksClient)

	logger.Op.Info("Successfully initialized GCP Compute API clients.")
	return &Clients{
		DiskClient:     diskClient,
		SnapshotClient: snapshotClient,
		ComputeClient:  computeClient,
		Zones:          zonesClient,
		Regions:        regionsClient,
	}, nil
}

func (c *Clients) Close() {
	logger.Op.Debug("Closing GCP Compute API clients...")
	if c.DiskClient != nil {
		_ = c.DiskClient.Close()
	}
	if c.SnapshotClient != nil {
		_ = c.SnapshotClient.Close()
	}
	if c.ComputeClient != nil {
		_ = c.ComputeClient.Close()
	}
	if c.Zones != nil {
		_ = c.Zones.Close()
	}
	if c.Regions != nil {
		_ = c.Regions.Close()
	}
	logger.Op.Debug("GCP Compute API clients closed.")
}

func getDefaultClientOptions() []option.ClientOption {
	return nil
}
