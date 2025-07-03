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

# Target instance with specific labels
module "target_instance" {
  source = "../../modules/gce_with_disks"
  
  resource_prefix = var.resource_prefix
  project_id      = var.project_id
  zone            = var.zone
  instance_name   = "target-instance"
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
      size = 100
      type = var.disk_type
    }
  ]
  
  random_data_size_mb = var.random_data_size_mb
  
  # This instance will have the test labels
  labels = merge(
    {
      purpose     = "integration-test"
      test_type   = "label-filter"
      role        = "target"
    },
    var.test_instance_labels
  )
}

# Decoy instance without matching labels
module "decoy_instance" {
  source = "../../modules/gce_with_disks"
  
  resource_prefix = var.resource_prefix
  project_id      = var.project_id
  zone            = var.zone
  instance_name   = "decoy-instance"
  machine_type    = var.machine_type
  boot_disk_type  = var.boot_disk_type
  
  attached_disks = [
    {
      name = "decoy-disk-1"
      size = 100
      type = var.disk_type
    }
  ]
  
  random_data_size_mb = var.random_data_size_mb
  
  # This instance will have different labels
  labels = {
    purpose     = "integration-test"
    test_type   = "label-filter"
    role        = "decoy"
    environment = "ignore-me"
    app         = "decoy-app"
  }
}