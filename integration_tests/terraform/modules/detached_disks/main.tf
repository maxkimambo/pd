terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

locals {
  zonal_disks    = [for disk in var.disks : disk if !disk.regional]
  regional_disks = [for disk in var.disks : disk if disk.regional]
}

resource "google_compute_disk" "zonal" {
  count   = length(local.zonal_disks)
  project = var.project_id
  zone    = var.zone
  name    = "${var.resource_prefix}-${local.zonal_disks[count.index].name}"
  type    = local.zonal_disks[count.index].type
  size    = local.zonal_disks[count.index].size
  labels  = merge(var.labels, { resource_prefix = var.resource_prefix })
}

resource "google_compute_region_disk" "regional" {
  count   = length(local.regional_disks)
  project = var.project_id
  region  = var.region
  name    = "${var.resource_prefix}-${local.regional_disks[count.index].name}"
  type    = local.regional_disks[count.index].type
  size    = local.regional_disks[count.index].size
  labels  = merge(var.labels, { resource_prefix = var.resource_prefix })
  
  replica_zones = [
    "${var.region}-a",
    "${var.region}-b"
  ]
}