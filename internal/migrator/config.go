package migrator

import "github.com/maxkimambo/pd/internal/gcp" 

type Config struct {
	ProjectID      string
	TargetDiskType string
	LabelFilter    string 
	KmsKey         string 
	KmsKeyRing     string
	KmsLocation    string
	KmsProject     string
	KmsParams      *gcp.SnapshotKmsParams 
	Region         string
	Zone           string
	SkipConfirm    bool 
	AutoApproveAll bool 
	MaxConcurrency int
	RetainName     bool
	Debug          bool

}

func (c *Config) PopulateKmsParams() *gcp.SnapshotKmsParams {
	if c.KmsKey != "" {
		kmsProject := c.KmsProject
		if kmsProject == "" {
			kmsProject = c.ProjectID 
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

func (c *Config) Location() string {
	if c.Zone != "" {
		return c.Zone
	}
	return c.Region
}
