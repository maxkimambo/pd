terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

resource "google_compute_disk" "attached" {
  count   = length(var.attached_disks)
  project = var.project_id
  zone    = var.zone
  name    = "${var.resource_prefix}-${var.attached_disks[count.index].name}"
  type    = var.attached_disks[count.index].type
  size    = var.attached_disks[count.index].size
  labels  = merge(var.labels, { resource_prefix = var.resource_prefix })
}

resource "google_compute_instance" "default" {
  project      = var.project_id
  zone         = var.zone
  name         = "${var.resource_prefix}-${var.instance_name}"
  machine_type = var.machine_type
  
  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
      size  = var.boot_disk_size
      type  = var.boot_disk_type
    }
  }
  
  dynamic "attached_disk" {
    for_each = google_compute_disk.attached
    content {
      source      = attached_disk.value.id
      device_name = attached_disk.value.name
    }
  }
  
  network_interface {
    network = "default"
    access_config {}
  }
  
  labels = merge(var.labels, { resource_prefix = var.resource_prefix })
  
  metadata = {
    startup-script = <<-EOF
      #!/bin/bash
      echo "Instance started at $(date)" > /tmp/startup-time.txt
    EOF
  }
}