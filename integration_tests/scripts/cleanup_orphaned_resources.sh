#!/bin/bash

set -euo pipefail

PROJECT_ID="${GCP_PROJECT_ID:-}"
MAX_AGE_HOURS="${MAX_AGE_HOURS:-2}"
DRY_RUN="${DRY_RUN:-false}"

if [ -z "$PROJECT_ID" ]; then
    echo "Error: GCP_PROJECT_ID environment variable must be set"
    exit 1
fi

echo "Cleaning up orphaned test resources in project: $PROJECT_ID"
echo "Resources older than ${MAX_AGE_HOURS} hours will be deleted"
echo "Dry run: $DRY_RUN"
echo "---"

current_timestamp=$(date +%s)
max_age_seconds=$((MAX_AGE_HOURS * 3600))

# Function to check if resource is old enough to delete
is_old_enough() {
    local created_timestamp="$1"
    local created_seconds=$(date -d "$created_timestamp" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "$created_timestamp" +%s 2>/dev/null || echo 0)
    local age_seconds=$((current_timestamp - created_seconds))
    
    [ $age_seconds -gt $max_age_seconds ]
}

# Clean up compute instances
echo "Checking compute instances..."
gcloud compute instances list \
    --project="$PROJECT_ID" \
    --filter="labels.purpose=integration-test" \
    --format="json" | jq -r '.[] | "\(.name) \(.zone) \(.creationTimestamp)"' | \
while read -r name zone created; do
    zone_short=$(echo "$zone" | rev | cut -d'/' -f1 | rev)
    if is_old_enough "$created"; then
        echo "Found old instance: $name in zone $zone_short (created: $created)"
        if [ "$DRY_RUN" != "true" ]; then
            echo "  Deleting instance..."
            gcloud compute instances delete "$name" \
                --zone="$zone_short" \
                --project="$PROJECT_ID" \
                --quiet || echo "  Failed to delete instance $name"
        else
            echo "  [DRY RUN] Would delete instance"
        fi
    fi
done

# Clean up disks
echo -e "\nChecking disks..."
gcloud compute disks list \
    --project="$PROJECT_ID" \
    --filter="labels.purpose=integration-test AND -users:*" \
    --format="json" | jq -r '.[] | "\(.name) \(.zone) \(.creationTimestamp)"' | \
while read -r name zone created; do
    zone_short=$(echo "$zone" | rev | cut -d'/' -f1 | rev)
    if is_old_enough "$created"; then
        echo "Found old disk: $name in zone $zone_short (created: $created)"
        if [ "$DRY_RUN" != "true" ]; then
            echo "  Deleting disk..."
            gcloud compute disks delete "$name" \
                --zone="$zone_short" \
                --project="$PROJECT_ID" \
                --quiet || echo "  Failed to delete disk $name"
        else
            echo "  [DRY RUN] Would delete disk"
        fi
    fi
done

# Clean up regional disks
echo -e "\nChecking regional disks..."
gcloud compute disks list \
    --project="$PROJECT_ID" \
    --filter="labels.purpose=integration-test AND region:* AND -users:*" \
    --format="json" | jq -r '.[] | "\(.name) \(.region) \(.creationTimestamp)"' | \
while read -r name region created; do
    region_short=$(echo "$region" | rev | cut -d'/' -f1 | rev)
    if is_old_enough "$created"; then
        echo "Found old regional disk: $name in region $region_short (created: $created)"
        if [ "$DRY_RUN" != "true" ]; then
            echo "  Deleting regional disk..."
            gcloud compute disks delete "$name" \
                --region="$region_short" \
                --project="$PROJECT_ID" \
                --quiet || echo "  Failed to delete regional disk $name"
        else
            echo "  [DRY RUN] Would delete regional disk"
        fi
    fi
done

# Clean up snapshots
echo -e "\nChecking snapshots..."
gcloud compute snapshots list \
    --project="$PROJECT_ID" \
    --filter="labels.purpose=integration-test OR name:test-*" \
    --format="json" | jq -r '.[] | "\(.name) \(.creationTimestamp)"' | \
while read -r name created; do
    if is_old_enough "$created"; then
        echo "Found old snapshot: $name (created: $created)"
        if [ "$DRY_RUN" != "true" ]; then
            echo "  Deleting snapshot..."
            gcloud compute snapshots delete "$name" \
                --project="$PROJECT_ID" \
                --quiet || echo "  Failed to delete snapshot $name"
        else
            echo "  [DRY RUN] Would delete snapshot"
        fi
    fi
done

echo -e "\nCleanup complete!"