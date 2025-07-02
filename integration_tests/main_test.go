package integration

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

var (
	keepResources bool
)

func TestMain(m *testing.M) {
	flag.BoolVar(&keepResources, "keep-resources", false, "Keep test resources after test completion (for debugging)")
	flag.Parse()

	if os.Getenv("GCP_PROJECT_ID") == "" {
		fmt.Println("GCP_PROJECT_ID environment variable must be set")
		os.Exit(1)
	}

	if _, err := os.Stat("../pd"); err != nil {
		fmt.Println("pd binary not found. Please build the project first with 'go build -o pd main.go'")
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}