package validation

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
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
