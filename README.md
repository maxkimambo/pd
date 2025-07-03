# PD - Google Cloud Persistent Disk Migration Tool

Bulk migrate Google Cloud persistent disks between disk types with automatic instance management.

## Installation

```bash
make build 

# Add binary to PATH
export PATH=$PATH:$(pwd)/bin
```

## Quick Start

```bash
# Migrate detached disks
pd migrate disk --project my-project --zone us-central1-a --target-disk-type hyperdisk-balanced

# Migrate attached disks (with instance restart)
pd migrate compute --project my-project --zone us-central1-a --instances vm1,vm2 --target-disk-type pd-ssd
```

## Command-Line Flags

### Required Flags

- `--project` / `-p` - GCP project ID
- `--target-disk-type` / `-t` - Target disk type (pd-standard, pd-ssd, pd-balanced, hyperdisk-balanced, hyperdisk-extreme, hyperdisk-throughput)

### Location (Required: Choose One)

- `--zone` - GCP zone for zonal resources (e.g., us-central1-a)
- `--region` - GCP region for regional resources (e.g., us-central1)

### Migration Options

- `--instances` - Comma-separated instance names or "*" for all (compute command only, required)
- `--label` - Filter by label key=value (e.g., env=staging)
- `--auto-approve` - Skip all confirmation prompts
- `--retain-name` - Reuse original disk name by deleting original (default: true)
- `--dry-run` - Preview changes without making them
- `--concurrency` - Concurrent operations (disk: 1-200, default 10; compute: 1-50, default 5)

### Disk Performance

- `--iops` - IOPS for new disk (3000-350000, default: 3000)
- `--throughput` - Throughput MB/s (140-5000, default: 140)
- `--storage-pool-id` / `-s` - Storage pool ID for hyperdisk

### KMS Encryption

- `--kms-key` - KMS key name for snapshot encryption
- `--kms-keyring` - KMS keyring name (required with kms-key)
- `--kms-location` - KMS location (required with kms-key)
- `--kms-project` - KMS project (defaults to --project)

### Logging Options

- `--debug` - Enable debug logging
- `--verbose` / `-v` - Enable verbose logging
- `--quiet` / `-q` - Suppress non-error output
- `--json` - Output logs as JSON format

### Environment Variables

- `LOG_MODE` - Set to "quiet" or "verbose"
- `LOG_FORMAT` - Set to "json" or "text" (default)

## Examples

```bash
# Migrate detached disks with performance settings
pd migrate disk \
  --project prod-project \
  --zone us-east1-b \
  --target-disk-type hyperdisk-balanced \
  --label team=backend \
  --iops 10000 \
  --throughput 250 \
  --auto-approve

# Migrate instance disks with KMS encryption
pd migrate compute \
  --project my-project \
  --zone us-central1-a \
  --instances vm1 \
  --target-disk-type pd-ssd \
  --kms-key my-key \
  --kms-keyring my-keyring \
  --kms-location us-central1
```
