package migrator

import "github.com/maxkimambo/pd/internal/gcp" // Import gcp types if needed later

// Config holds the configuration parameters for the migration process,
// typically parsed from command-line flags.
type Config struct {
	ProjectID      string
	TargetDiskType string
	LabelFilter    string // Format: "key=value"
	KmsKey         string // Added KMS fields
	KmsKeyRing     string
	KmsLocation    string
	KmsProject     string
	KmsParams      *gcp.SnapshotKmsParams // Use the struct from gcp package
	Region         string
	Zone           string
	SkipConfirm    bool // Corresponds to --yes flag (or --auto-approve)
	AutoApproveAll bool // Corresponds to --auto-approve flag
	MaxConcurrency int
	RetainName     bool
	Debug          bool

	// Internal fields (populated during execution)
	// gcpClient *gcp.Clients // Might add client here later
}

// PopulateKmsParams creates SnapshotKmsParams from Config fields if KMS is used.
func (c *Config) PopulateKmsParams() *gcp.SnapshotKmsParams {
	// Check if any KMS flag was actually set in the command
	// This requires accessing the original flags or passing them differently.
	// For now, assume if KmsKey is non-empty, KMS is intended.
	if c.KmsKey != "" {
		kmsProject := c.KmsProject
		if kmsProject == "" {
			kmsProject = c.ProjectID // Default to main project ID
		}
		return &gcp.SnapshotKmsParams{
			KmsKey:      c.KmsKey,
			KmsKeyRing:  c.KmsKeyRing,
			KmsLocation: c.KmsLocation,
			KmsProject:  kmsProject,
		}
	}
	return nil
}

// Location returns the specified zone or region.
func (c *Config) Location() string {
	if c.Zone != "" {
		return c.Zone
	}
	return c.Region
}
