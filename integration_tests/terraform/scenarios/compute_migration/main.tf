terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
}

module "test_instance" {
  source = "../../modules/gce_with_disks"
  
  resource_prefix = var.resource_prefix
  project_id      = var.project_id
  zone            = var.zone
  instance_name   = "test-migration-instance"
  machine_type    = var.machine_type
  boot_disk_type  = var.boot_disk_type
  
  attached_disks = [
    {
      name = "data-disk-1"
      size = 100
      type = var.disk_type
    },
    {
      name = "data-disk-2"
      size = 200
      type = var.disk_type
    }
  ]
  
  labels = {
    purpose     = "integration-test"
    test_type   = "compute-migration"
    environment = "test"
  }
}