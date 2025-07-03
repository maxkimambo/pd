package testutil

import (
	"os"
	"path/filepath"
)

// GetPDBinaryPath returns the path to the pd binary for integration tests.
// It checks multiple locations in order of preference:
// 1. Current directory (./pd) - where Makefile copies it
// 2. Parent directory (../pd) - for backward compatibility
// 3. bin directory (../bin/pd) - where make build creates it
func GetPDBinaryPath() string {
	// Check current directory first (where Makefile copies the binary)
	if _, err := os.Stat("pd"); err == nil {
		return "./pd"
	}

	// Check parent directory (backward compatibility)
	if _, err := os.Stat("../pd"); err == nil {
		return "../pd"
	}

	// Check bin directory
	binPath := filepath.Join("..", "bin", "pd")
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	// Default to current directory (will fail if binary doesn't exist)
	return "./pd"
}