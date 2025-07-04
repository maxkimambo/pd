#!/bin/bash

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <test-name> [additional-go-test-flags]"
    echo "Example: $0 TestComputeMigration/migrate_pd_balanced_to_hyperdisk_balanced_c3 -v"
    exit 1
fi

TEST_NAME="$1"
shift

PROJECT_ID="${GCP_PROJECT_ID:-}"

if [ -z "$PROJECT_ID" ]; then
    echo "Error: GCP_PROJECT_ID environment variable must be set"
    exit 1
fi

# Get the script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
INTEGRATION_TEST_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Build and copy binary to integration tests directory
echo "Building pd binary..."
cd "$PROJECT_ROOT"
make build-integration

echo "Running test: $TEST_NAME"
cd "$INTEGRATION_TEST_DIR"

go test -run "$TEST_NAME" -timeout 30m "$@" .