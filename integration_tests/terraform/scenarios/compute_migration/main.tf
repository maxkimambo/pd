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
  
  attached_disks = [
    {
      name = "data-disk-1"
      size = 10
      type = "pd-standard"
    },
    {
      name = "data-disk-2"
      size = 20
      type = "pd-standard"
    }
  ]
  
  labels = {
    purpose     = "integration-test"
    test_type   = "compute-migration"
    environment = "test"
  }
}