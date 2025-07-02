# Integration Tests

This directory contains integration tests for the GCP disk migration tool. These tests create real GCP resources using Terraform, run migration commands against them, and verify the results.

## Prerequisites

1. GCP Project with billing enabled
2. Service account with the following roles:
   - Compute Admin
   - Service Account User
3. Terraform installed (>= 1.0)
4. gcloud CLI installed and configured
5. Go test dependencies: `go get github.com/stretchr/testify`

## Environment Variables

Set the following environment variables before running tests:

```bash
export GCP_PROJECT_ID="your-test-project-id"
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"
```

## Building the Binary

Before running integration tests, build the pd binary:

```bash
cd ..
go build -o pd main.go
```

## Running Tests

Run all integration tests:
```bash
go test -v ./integration_tests/...
```

Run specific test:
```bash
go test -v ./integration_tests/... -run TestComputeMigration
```

Run tests in parallel:
```bash
go test -v -parallel 4 ./integration_tests/...
```

Skip integration tests (useful for unit tests only):
```bash
go test -short ./...
```

Keep resources after test (for debugging):
```bash
go test -v ./integration_tests/... -keep-resources
```

## Test Structure

```
integration_tests/
├── terraform/
│   ├── modules/
│   │   ├── gce_with_disks/     # Module for creating instances with attached disks
│   │   └── detached_disks/     # Module for creating detached disks
│   └── scenarios/
│       ├── compute_migration/   # Scenario for testing compute instance migration
│       └── disk_migration/      # Scenario for testing detached disk migration
├── internal/
│   ├── terraform/              # Terraform wrapper functions
│   └── gcloud/                 # gcloud wrapper functions
├── compute_migration_test.go   # Tests for migrate compute command
├── disk_migration_test.go      # Tests for migrate disk command
└── main_test.go               # Test setup and configuration
```

## Test Isolation

Each test run creates resources with a unique prefix (based on timestamp) to ensure isolation. This allows tests to run in parallel without conflicts.

## Cleanup

Tests automatically clean up resources after completion using Terraform destroy. If a test fails or is interrupted, you may need to manually clean up resources:

```bash
# List test resources
gcloud compute instances list --filter="labels.purpose=integration-test"
gcloud compute disks list --filter="labels.purpose=integration-test"

# Manual cleanup if needed
gcloud compute instances delete INSTANCE_NAME --zone=ZONE
gcloud compute disks delete DISK_NAME --zone=ZONE
```

## CI/CD Integration

To run these tests in CI/CD:

1. Set up a dedicated GCP project for testing
2. Create a service account with necessary permissions
3. Store the service account key as a secret in your CI/CD system
4. Set environment variables in CI/CD pipeline
5. Run tests as part of your build process

Example GitHub Actions workflow:

```yaml
- name: Run Integration Tests
  env:
    GCP_PROJECT_ID: ${{ secrets.GCP_TEST_PROJECT_ID }}
    GOOGLE_APPLICATION_CREDENTIALS: /tmp/gcp-key.json
  run: |
    echo '${{ secrets.GCP_SA_KEY }}' > /tmp/gcp-key.json
    go test -v ./integration_tests/...
```

## Adding New Tests

1. Create new Terraform scenarios in `terraform/scenarios/` if needed
2. Add test cases to existing test files or create new test files
3. Follow the existing pattern of setup, execute, verify, and cleanup
4. Ensure proper error handling and resource cleanup