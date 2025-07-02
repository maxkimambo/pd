package validation

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
)

//go:embed compatibility_matrix.json
var compatibilityMatrixJSON []byte

// CompatibilityMatrix represents the disk type to machine type compatibility data
type CompatibilityMatrix struct {
	DiskTypes map[string]DiskTypeInfo `json:"diskTypes"`
}

// DiskTypeInfo contains the supported machine types for a specific disk type
type DiskTypeInfo struct {
	SupportedMachineTypes []string `json:"supportedMachineTypes"`
}

// ValidationResult represents the result of a compatibility check
type ValidationResult struct {
	Compatible bool
	Reason     string
}

var matrix *CompatibilityMatrix

// init loads the embedded compatibility matrix
func init() {
	var err error
	matrix, err = loadCompatibilityMatrix()
	if err != nil {
		// Fallback to basic compatibility if matrix fails to load
		matrix = getDefaultMatrix()
	}
}

// loadCompatibilityMatrix loads the embedded JSON compatibility matrix
func loadCompatibilityMatrix() (*CompatibilityMatrix, error) {
	var cm CompatibilityMatrix
	err := json.Unmarshal(compatibilityMatrixJSON, &cm)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal compatibility matrix: %w", err)
	}
	return &cm, nil
}

// getDefaultMatrix provides a basic fallback compatibility matrix
func getDefaultMatrix() *CompatibilityMatrix {
	return &CompatibilityMatrix{
		DiskTypes: map[string]DiskTypeInfo{
			"pd-standard": {
				SupportedMachineTypes: []string{"e2-micro", "f1-micro", "n2-standard-8"},
			},
			"pd-balanced": {
				SupportedMachineTypes: []string{"e2-micro", "n2-standard-8"},
			},
		},
	}
}

// IsCompatible checks if a machine type supports a given disk type
func IsCompatible(machineType, diskType string) ValidationResult {
	// Normalize inputs
	machineType = strings.ToLower(strings.TrimSpace(machineType))
	diskType = strings.ToLower(strings.TrimSpace(diskType))

	if machineType == "" {
		return ValidationResult{
			Compatible: false,
			Reason:     "machine type cannot be empty",
		}
	}

	if diskType == "" {
		return ValidationResult{
			Compatible: false,
			Reason:     "disk type cannot be empty",
		}
	}

	// Check if disk type exists in our matrix
	diskInfo, exists := matrix.DiskTypes[diskType]
	if !exists {
		return ValidationResult{
			Compatible: false,
			Reason:     fmt.Sprintf("unknown disk type: %s", diskType),
		}
	}

	// Check if machine type is in the supported list for this disk type
	for _, supportedMachineType := range diskInfo.SupportedMachineTypes {
		if supportedMachineType == machineType {
			return ValidationResult{
				Compatible: true,
				Reason:     fmt.Sprintf("machine type %s supports disk type %s", machineType, diskType),
			}
		}
	}

	return ValidationResult{
		Compatible: false,
		Reason:     fmt.Sprintf("machine type %s does not support disk type %s", machineType, diskType),
	}
}

// GetSupportedDiskTypes returns all supported disk types for a given machine type
func GetSupportedDiskTypes(machineType string) []string {
	machineType = strings.ToLower(strings.TrimSpace(machineType))

	var supportedDiskTypes []string

	// Iterate through all disk types and check if they support this machine type
	for diskType, diskInfo := range matrix.DiskTypes {
		for _, supportedMachineType := range diskInfo.SupportedMachineTypes {
			if supportedMachineType == machineType {
				supportedDiskTypes = append(supportedDiskTypes, diskType)
				break
			}
		}
	}

	return supportedDiskTypes
}

// GetAllMachineTypes returns all machine types mentioned in the compatibility matrix
func GetAllMachineTypes() []string {
	machineTypeSet := make(map[string]bool)

	// Collect all unique machine types from all disk types
	for _, diskInfo := range matrix.DiskTypes {
		for _, machineType := range diskInfo.SupportedMachineTypes {
			machineTypeSet[machineType] = true
		}
	}

	// Convert set to slice
	var machineTypes []string
	for machineType := range machineTypeSet {
		machineTypes = append(machineTypes, machineType)
	}

	return machineTypes
}

// GetAllDiskTypes returns all disk types in the compatibility matrix
func GetAllDiskTypes() []string {
	var diskTypes []string
	for diskType := range matrix.DiskTypes {
		diskTypes = append(diskTypes, diskType)
	}
	return diskTypes
}

// ValidateProjectID validates a GCP project ID
func ValidateProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}
	// GCP project IDs must be 6-30 characters, lowercase letters, digits, or hyphens
	if len(projectID) < 6 || len(projectID) > 30 {
		return fmt.Errorf("project ID must be between 6 and 30 characters")
	}
	return nil
}

// ValidateConcurrency validates the concurrency value is within an acceptable range.
func ValidateConcurrency(concurrency int, max int) error {
	if concurrency < 1 || concurrency > max {
		return fmt.Errorf("concurrency must be between 1 and %d, got %d", max, concurrency)
	}
	return nil
}

// ValidateThroughput validates throughput value is within acceptable range
func ValidateThroughput(throughput int64) error {
	if throughput < 140 || throughput > 5000 {
		return fmt.Errorf("throughput must be between 140 and 5000 MB/s, got %d", throughput)
	}
	return nil
}

// ValidateIOPS validates IOPS value is within acceptable range
func ValidateIOPS(iops int64) error {
	if iops < 3000 || iops > 350000 {
		return fmt.Errorf("IOPS must be between 3000 and 350,000, got %d", iops)
	}
	return nil
}

// ValidateLabelFilter validates the label filter format
func ValidateLabelFilter(labelFilter string) error {
	if labelFilter == "" {
		return nil // empty is valid
	}
	if !strings.Contains(labelFilter, "=") {
		return fmt.Errorf("invalid label format: %s. Expected key=value", labelFilter)
	}
	return nil
}

// ValidateKMSConfig validates KMS configuration completeness
func ValidateKMSConfig(kmsKey, kmsKeyRing, kmsLocation string) error {
	if kmsKey == "" {
		return nil // KMS is optional
	}
	if kmsKeyRing == "" || kmsLocation == "" {
		return fmt.Errorf("--kms-keyring and --kms-location are required when --kms-key is specified")
	}
	return nil
}

// ValidateLocationFlags validates that exactly one of zone or region is specified
func ValidateLocationFlags(zone, region string) error {
	if (zone == "" && region == "") || (zone != "" && region != "") {
		return fmt.Errorf("exactly one of --zone or --region must be specified")
	}
	return nil
}

// ValidateGCPAuthentication validates that the user has proper GCP authentication configured
func ValidateGCPAuthentication() error {
	ctx := context.Background()
	
	// Check if GOOGLE_APPLICATION_CREDENTIALS is set
	if credPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credPath != "" {
		// Verify the file exists
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			return fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS points to non-existent file: %s", credPath)
		}
		// Try to load the credentials
		_, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/compute")
		if err != nil {
			return fmt.Errorf("failed to load credentials from GOOGLE_APPLICATION_CREDENTIALS: %w", err)
		}
		return nil
	}
	
	// Try to find default credentials (gcloud auth, metadata service, etc.)
	_, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/compute")
	if err != nil {
		return fmt.Errorf(`no valid GCP authentication found. Please authenticate using one of the following methods:
  1. Set GOOGLE_APPLICATION_CREDENTIALS environment variable to point to a service account key file:
     export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"
  
  2. Use gcloud to authenticate:
     gcloud auth application-default login
  
  3. Run this application on a GCP environment (GCE, GKE, Cloud Run) with proper IAM roles

Error: %w`, err)
	}
	
	return nil
}
