package gcp

import (
	"context"
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

// Clients holds the initialized GCP Compute clients.
type Clients struct {
	Disks     *compute.DisksClient
	Snapshots *compute.SnapshotsClient
	Zones     *compute.ZonesClient
	Regions   *compute.RegionsClient
	// Add other clients like ImagesClient if needed later
}

// NewClients creates and returns new GCP Compute API clients.
// It uses Application Default Credentials (ADC) for authentication.
func NewClients(ctx context.Context) (*Clients, error) {
	logrus.Debug("Initializing GCP Compute API clients...")

	// Create Disks client
	disksClient, err := compute.NewDisksRESTClient(ctx) // Using REST for potentially broader compatibility/simpler setup
	if err != nil {
		return nil, fmt.Errorf("failed to create compute Disks client: %w", err)
	}
	logrus.Debug("Disks client initialized.")

	// Create Snapshots client
	snapshotsClient, err := compute.NewSnapshotsRESTClient(ctx)
	if err != nil {
		// Attempt to close already created clients on subsequent failure
		disksClient.Close()
		return nil, fmt.Errorf("failed to create compute Snapshots client: %w", err)
	}
	logrus.Debug("Snapshots client initialized.")

	// Create Zones client (might be useful for validation or getting zone details)
	zonesClient, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		disksClient.Close()
		snapshotsClient.Close()
		return nil, fmt.Errorf("failed to create compute Zones client: %w", err)
	}
	logrus.Debug("Zones client initialized.")

	// Create Regions client (might be useful for validation or getting region details)
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

// Close closes all the initialized clients.
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

// Helper function to get default client options (useful if needing options explicitly)
func getDefaultClientOptions() []option.ClientOption {
	// Example: return []option.ClientOption{option.WithCredentialsFile("path/to/keyfile.json")}
	// By default, New...RESTClient uses ADC, so often no explicit options are needed.
	return nil
}
