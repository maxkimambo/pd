terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  zone    = var.zone
  credentials = (file("../key.json"))
}

locals {
  disks_to_create = merge([
    for i_idx in range(var.instance_count) : {
      for d_idx in range(var.disk_count_per_instance) :
      "instance-${i_idx}-disk-${d_idx}" => {
        name                   = "${var.disk_name_prefix}-i${i_idx}-d${d_idx}"
        instance_index         = i_idx
        disk_index_on_instance = d_idx
      }
    }
  ]...)
}

resource "google_compute_disk" "extra_disks" {
  for_each = local.disks_to_create

  name    = each.value.name
  type    = "hyperdisk-balanced"
  zone    = var.zone
  project = var.project_id
  size    = var.disk_size_gb

  lifecycle {
    prevent_destroy = true
  }

  labels = {
    created_by             = "terraform"
    instance_association   = "instance-${each.value.instance_index}"
    disk_index_on_instance = format("%d", each.value.disk_index_on_instance)
  }
}

resource "google_compute_instance" "gce_instances" {
  count        = var.instance_count
  project      = var.project_id
  zone         = var.zone
  name         = "${var.instance_name_prefix}-${count.index}"
  machine_type = "c3-standard-4"

  boot_disk {
    initialize_params {
      image = var.boot_disk_image
    }
  }

  network_interface {
    network = "default"
  }

  dynamic "attached_disk" {
    for_each = range(var.disk_count_per_instance)
    content {
      source      = google_compute_disk.extra_disks["instance-${count.index}-disk-${attached_disk.key}"].self_link
      device_name = "data-disk-${attached_disk.key}"
    }
  }

  allow_stopping_for_update = true

  labels = {
    created_by     = "terraform"
    instance_index = format("%d", count.index)
  }

  scheduling {
    automatic_restart   = true
    on_host_maintenance = "MIGRATE"
  }
}
