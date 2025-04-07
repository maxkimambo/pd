package migrator

import (
	"bufio"
	"context"
	"fmt"
	"gcp-disk-migrator/internal/gcp"
	"os"
	"strings"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
)

// DiscoverDisks performs Phase 1: finding eligible disks and confirming with the user.
func DiscoverDisks(ctx context.Context, config *Config, gcpClient *gcp.Clients) ([]*computepb.Disk, error) {
	logrus.Info("--- Phase 1: Discovery ---")

	location := config.Location()
	logrus.Infof("Listing detached disks in %s (Project: %s)", location, config.ProjectID)
	if config.LabelFilter != "" {
		logrus.Infof("Applying label filter: %s", config.LabelFilter)
	}

	// List detached disks
	disksToMigrate, err := gcpClient.ListDetachedDisks(ctx, config.ProjectID, location, config.LabelFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list detached disks: %w", err)
	}

	if len(disksToMigrate) == 0 {
		logrus.Info("No detached disks found matching the criteria.")
		return []*computepb.Disk{}, nil // Return empty slice, not an error
	}

	logrus.Infof("Found %d detached disk(s) matching criteria:", len(disksToMigrate))
	for i, disk := range disksToMigrate {
		// Extract zone from self-link for display
		zone := "unknown"
		if disk.Zone != nil {
			parts := strings.Split(disk.GetZone(), "/")
			zone = parts[len(parts)-1]
		}
		fmt.Printf("  %d. %s (Zone: %s, Type: %s, Size: %d GB)\n",
			i+1,
			disk.GetName(),
			zone,
			getShortDiskTypeName(disk.GetType()),
			disk.GetSizeGb(),
		)
	}

	// Confirmation prompt
	if !config.AutoApproveAll && !config.SkipConfirm {
		fmt.Printf("\nProceed with migrating these %d disk(s) to type '%s'? (yes/no): ", len(disksToMigrate), config.TargetDiskType)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read user confirmation: %w", err)
		}
		input = strings.ToLower(strings.TrimSpace(input))
		if input != "yes" {
			logrus.Info("Migration cancelled by user.")
			return []*computepb.Disk{}, nil // User cancelled, return empty slice
		}
		logrus.Info("User confirmed. Proceeding with migration.")
	} else if config.SkipConfirm || config.AutoApproveAll {
		logrus.Warn("Skipping user confirmation due to --yes or --auto-approve flag.")
	}

	logrus.Info("--- Discovery Phase Complete ---")
	return disksToMigrate, nil
}

// Helper to get the short name (e.g., pd-ssd) from the full disk type URL
func getShortDiskTypeName(typeURL string) string {
	if typeURL == "" {
		return "unknown"
	}
	parts := strings.Split(typeURL, "/")
	return parts[len(parts)-1]
}
