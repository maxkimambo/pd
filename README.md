# PD - Google Cloud Persistent Disk Migration Tool

A CLI tool for bulk migrating Google Cloud persistent disks from one type to another, supporting both detached disks and disks attached to GCE instances.

## Features

- Migrate detached persistent disks between different disk types
- Migrate disks attached to running GCE instances with automatic instance management
- Support for KMS encryption on snapshots
- Concurrent processing with configurable limits
- Interactive prompts with auto-approve mode
- Detailed progress tracking and reporting
- Cleanup of temporary snapshots

## Installation

```bash
# Build from source
make build

# Run tests
make test

# Install
make install
```

## Usage

### Migrate Detached Disks

```bash
# Migrate all detached disks in a zone
pd migrate disk --project my-project --zone us-central1-a --target-disk-type hyperdisk-balanced

# Migrate with label filter
pd migrate disk --project my-project --zone us-central1-a --target-disk-type pd-ssd --label env=staging

# Auto-approve all prompts
pd migrate disk --project my-project --zone us-central1-a --target-disk-type pd-ssd --auto-approve
```

### Migrate Attached Disks (GCE Instances)

```bash
# Migrate disks for specific instances
pd migrate compute --project my-project --zone us-central1-a --instances vm1,vm2 --target-disk-type hyperdisk-balanced

# Migrate all instances in a zone
pd migrate compute --project my-project --zone us-central1-a --instances "*" --target-disk-type pd-ssd

# With custom IOPS and throughput
pd migrate compute --project my-project --zone us-central1-a --instances vm1 --target-disk-type hyperdisk-balanced --iops 10000 --throughput 250
```

## Logging

The tool uses a unified logging system that provides both user-friendly and detailed operational logs.

### Basic Usage

```go
import "github.com/maxkimambo/pd/internal/logger"

// Simple logging
logger.Info("Starting process")
logger.Error("An error occurred")
logger.Debug("Debug information")

// Formatted logging
logger.Infof("Processing %d items", count)
logger.Errorf("Failed to process: %v", err)

// Special purpose logging with emojis
logger.Starting("Beginning migration")    // 🚀 Beginning migration
logger.Success("Migration completed")     // ✅ Migration completed
logger.Snapshot("Creating snapshot")      // 📸 Creating snapshot
logger.Delete("Removing old disk")        // 🗑️ Removing old disk
logger.Create("Creating new disk")        // 💾 Creating new disk
logger.Cleanup("Cleaning up resources")   // 🧹 Cleaning up resources
```

### Structured Logging

```go
// Log with fields
logger.WithFieldsMap(map[string]interface{}{
    "disk": "my-disk",
    "zone": "us-central1-a",
    "size": 100,
}).Info("Processing disk")

// Log with single field
logger.WithField("instance", "vm-1").Info("Starting instance")
```

### Environment Variables

Control logging behavior at runtime:

```bash
# Set log verbosity
LOG_MODE=quiet pd migrate disk ...    # Only errors
LOG_MODE=verbose pd migrate disk ...  # All logs including debug

# Set output format
LOG_FORMAT=json pd migrate disk ...   # JSON output for parsing
LOG_FORMAT=text pd migrate disk ...   # Human-readable (default)
```

### Command Line Flags

```bash
# Verbose logging
pd migrate disk --verbose ...

# Quiet mode (errors only)
pd migrate disk --quiet ...

# JSON output
pd migrate disk --json ...
```

## Configuration

### Disk Migration Options

- `--target-disk-type`: Target disk type (required)
- `--iops`: IOPS for the new disk (default: 3000, min: 3000, max: 350000)
- `--throughput`: Throughput in MB/s (default: 140, min: 140, max: 5000)
- `--pool-id`: Storage pool ID for the new disks
- `--retain-name`: Reuse original disk name (default: true)

### KMS Encryption

```bash
pd migrate disk \
  --kms-key my-key \
  --kms-keyring my-keyring \
  --kms-location us-central1 \
  --kms-project my-kms-project
```

### Concurrency Control

- `--concurrency`: Number of disks to process concurrently (1-200, default: 10)
- `--max-concurrency`: For compute migrations (1-50, default: 5)

## Development

### Project Structure

```
pd/
├── cmd/                    # CLI commands
├── internal/
│   ├── gcp/               # Google Cloud client wrappers
│   ├── logger/            # Unified logging system
│   ├── migrator/          # Core migration logic
│   ├── taskmanager/       # Workflow execution engine
│   └── workflow/          # Migration workflow definitions
└── main.go               # Entry point
```

### Building

```bash
# Build binary
make build

# Run tests
make test

# Lint code
make lint

# Format code  
make fmt

# All checks
make all
```

## License

[License information here]