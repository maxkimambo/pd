package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCompatible(t *testing.T) {
	tests := []struct {
		name        string
		machineType string
		diskType    string
		expected    ValidationResult
	}{
		{
			name:        "N2 standard 8 with pd-standard (compatible)",
			machineType: "n2-standard-8",
			diskType:    "pd-standard",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-8 supports disk type pd-standard",
			},
		},
		{
			name:        "N2 standard 80 with hyperdisk-extreme (compatible)",
			machineType: "n2-standard-80",
			diskType:    "hyperdisk-extreme",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-80 supports disk type hyperdisk-extreme",
			},
		},
		{
			name:        "N2 standard 8 with hyperdisk-extreme (incompatible)",
			machineType: "n2-standard-8",
			diskType:    "hyperdisk-extreme",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type n2-standard-8 does not support disk type hyperdisk-extreme",
			},
		},
		{
			name:        "E2 micro with pd-balanced (compatible)",
			machineType: "e2-micro",
			diskType:    "pd-balanced",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type e2-micro supports disk type pd-balanced",
			},
		},
		{
			name:        "E2 micro with hyperdisk-extreme (incompatible)",
			machineType: "e2-micro",
			diskType:    "hyperdisk-extreme",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type e2-micro does not support disk type hyperdisk-extreme",
			},
		},
		{
			name:        "F1 micro with pd-standard (compatible)",
			machineType: "f1-micro",
			diskType:    "pd-standard",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type f1-micro supports disk type pd-standard",
			},
		},
		{
			name:        "F1 micro with pd-balanced (incompatible)",
			machineType: "f1-micro",
			diskType:    "pd-balanced",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type f1-micro does not support disk type pd-balanced",
			},
		},
		{
			name:        "C3 standard 88 with hyperdisk-extreme (compatible)",
			machineType: "c3-standard-88",
			diskType:    "hyperdisk-extreme",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type c3-standard-88 supports disk type hyperdisk-extreme",
			},
		},
		{
			name:        "C3 standard 4 with hyperdisk-extreme (incompatible)",
			machineType: "c3-standard-4",
			diskType:    "hyperdisk-extreme",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type c3-standard-4 does not support disk type hyperdisk-extreme",
			},
		},
		{
			name:        "C3 standard 4 with hyperdisk-balanced (compatible)",
			machineType: "c3-standard-4",
			diskType:    "hyperdisk-balanced",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type c3-standard-4 supports disk type hyperdisk-balanced",
			},
		},
		{
			name:        "N1 standard 8 with hyperdisk-balanced (incompatible)",
			machineType: "n1-standard-8",
			diskType:    "hyperdisk-balanced",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type n1-standard-8 does not support disk type hyperdisk-balanced",
			},
		},
		{
			name:        "Unknown machine type",
			machineType: "unknown-type",
			diskType:    "pd-standard",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type unknown-type does not support disk type pd-standard",
			},
		},
		{
			name:        "Unknown disk type",
			machineType: "n2-standard-8",
			diskType:    "unknown-disk",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "unknown disk type: unknown-disk",
			},
		},
		{
			name:        "Empty machine type",
			machineType: "",
			diskType:    "pd-standard",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type cannot be empty",
			},
		},
		{
			name:        "Empty disk type",
			machineType: "n2-standard-8",
			diskType:    "",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "disk type cannot be empty",
			},
		},
		{
			name:        "Case insensitive machine type",
			machineType: "N2-STANDARD-8",
			diskType:    "pd-standard",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-8 supports disk type pd-standard",
			},
		},
		{
			name:        "Case insensitive disk type",
			machineType: "n2-standard-8",
			diskType:    "PD-STANDARD",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-8 supports disk type pd-standard",
			},
		},
		{
			name:        "Whitespace handling",
			machineType: "  n2-standard-8  ",
			diskType:    "  pd-standard  ",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-8 supports disk type pd-standard",
			},
		},
		{
			name:        "Local SSD with N2 standard 16 (compatible)",
			machineType: "n2-standard-16",
			diskType:    "local-ssd",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-16 supports disk type local-ssd",
			},
		},
		{
			name:        "Local SSD with N2 standard 2 (incompatible - too small)",
			machineType: "n2-standard-2",
			diskType:    "local-ssd",
			expected: ValidationResult{
				Compatible: false,
				Reason:     "machine type n2-standard-2 does not support disk type local-ssd",
			},
		},
		{
			name:        "Hyperdisk throughput with N2 standard 32 (compatible)",
			machineType: "n2-standard-32",
			diskType:    "hyperdisk-throughput",
			expected: ValidationResult{
				Compatible: true,
				Reason:     "machine type n2-standard-32 supports disk type hyperdisk-throughput",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCompatible(tt.machineType, tt.diskType)
			assert.Equal(t, tt.expected.Compatible, result.Compatible)
			assert.Equal(t, tt.expected.Reason, result.Reason)
		})
	}
}

func TestGetSupportedDiskTypes(t *testing.T) {
	tests := []struct {
		name                string
		machineType         string
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name:                "N2 standard 8 supports basic disk types",
			machineType:         "n2-standard-8",
			expectedContains:    []string{"pd-standard", "pd-balanced", "pd-ssd", "hyperdisk-throughput"},
			expectedNotContains: []string{"hyperdisk-extreme"},
		},
		{
			name:                "N2 standard 80 supports hyperdisk extreme",
			machineType:         "n2-standard-80",
			expectedContains:    []string{"pd-standard", "pd-balanced", "pd-ssd", "hyperdisk-throughput", "hyperdisk-extreme", "local-ssd"},
			expectedNotContains: []string{},
		},
		{
			name:                "E2 micro limited disk support",
			machineType:         "e2-micro",
			expectedContains:    []string{"pd-standard", "pd-balanced"},
			expectedNotContains: []string{"hyperdisk-extreme", "hyperdisk-balanced", "local-ssd"},
		},
		{
			name:                "F1 micro minimal disk support",
			machineType:         "f1-micro",
			expectedContains:    []string{"pd-standard"},
			expectedNotContains: []string{"pd-balanced", "pd-ssd", "hyperdisk-extreme"},
		},
		{
			name:                "C3 standard 88 supports hyperdisk extreme",
			machineType:         "c3-standard-88",
			expectedContains:    []string{"hyperdisk-balanced", "hyperdisk-throughput", "hyperdisk-extreme", "local-ssd"},
			expectedNotContains: []string{"pd-standard", "pd-balanced"},
		},
		{
			name:                "C3 standard 4 supports hyperdisk balanced but not extreme",
			machineType:         "c3-standard-4",
			expectedContains:    []string{"hyperdisk-balanced", "hyperdisk-throughput", "local-ssd"},
			expectedNotContains: []string{"hyperdisk-extreme", "pd-standard"},
		},
		{
			name:                "Unknown machine type",
			machineType:         "unknown-type",
			expectedContains:    []string{},
			expectedNotContains: []string{"pd-standard", "pd-balanced"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSupportedDiskTypes(tt.machineType)

			for _, diskType := range tt.expectedContains {
				assert.Contains(t, result, diskType, "Expected %s to support %s", tt.machineType, diskType)
			}

			for _, diskType := range tt.expectedNotContains {
				assert.NotContains(t, result, diskType, "Expected %s to NOT support %s", tt.machineType, diskType)
			}
		})
	}
}

func TestGetAllMachineTypes(t *testing.T) {
	result := GetAllMachineTypes()

	// Check that we get some expected machine types
	assert.Contains(t, result, "n2-standard-8")
	assert.Contains(t, result, "e2-micro")
	assert.Contains(t, result, "c3-standard-88")
	assert.Contains(t, result, "f1-micro")

	// Check that the result is not empty
	assert.Greater(t, len(result), 0, "Should return at least some machine types")
}

func TestGetAllDiskTypes(t *testing.T) {
	result := GetAllDiskTypes()

	// Check that we get some expected disk types
	assert.Contains(t, result, "pd-standard")
	assert.Contains(t, result, "pd-balanced")
	assert.Contains(t, result, "hyperdisk-extreme")
	assert.Contains(t, result, "local-ssd")

	// Check that the result is not empty
	assert.Greater(t, len(result), 0, "Should return at least some disk types")
}

func TestCompatibilityMatrixLoading(t *testing.T) {
	// Test that the matrix was loaded correctly
	assert.NotNil(t, matrix, "Compatibility matrix should be loaded")
	assert.NotEmpty(t, matrix.DiskTypes, "Matrix should contain disk types")

	// Test some specific entries exist
	assert.Contains(t, matrix.DiskTypes, "pd-standard")
	assert.Contains(t, matrix.DiskTypes, "hyperdisk-extreme")

	// Test that pd-standard has expected machine types
	pdStandardInfo := matrix.DiskTypes["pd-standard"]
	assert.Contains(t, pdStandardInfo.SupportedMachineTypes, "n2-standard-8")
	assert.Contains(t, pdStandardInfo.SupportedMachineTypes, "e2-micro")
}

// TestValidationResultStruct tests the ValidationResult struct
func TestValidationResultStruct(t *testing.T) {
	result := ValidationResult{
		Compatible: true,
		Reason:     "test reason",
	}

	assert.True(t, result.Compatible)
	assert.Equal(t, "test reason", result.Reason)
}

// Benchmark tests
func BenchmarkIsCompatible(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsCompatible("n2-standard-8", "pd-standard")
	}
}

func BenchmarkGetSupportedDiskTypes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetSupportedDiskTypes("n2-standard-8")
	}
}

// Test edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	t.Run("Disk type with empty supported machine types list", func(t *testing.T) {
		// Test disk types that have empty arrays (pd-extreme, hyperdisk-ml, etc.)
		result := IsCompatible("n2-standard-8", "pd-extreme")
		assert.False(t, result.Compatible)
		assert.Contains(t, result.Reason, "does not support")
	})

	t.Run("GetSupportedDiskTypes with whitespace", func(t *testing.T) {
		result := GetSupportedDiskTypes("  n2-standard-8  ")
		assert.Greater(t, len(result), 0)
		assert.Contains(t, result, "pd-standard")
	})

	t.Run("Case sensitivity", func(t *testing.T) {
		result1 := IsCompatible("N2-STANDARD-8", "PD-STANDARD")
		result2 := IsCompatible("n2-standard-8", "pd-standard")
		assert.Equal(t, result1.Compatible, result2.Compatible)
		assert.Equal(t, result1.Reason, result2.Reason)
	})
}
