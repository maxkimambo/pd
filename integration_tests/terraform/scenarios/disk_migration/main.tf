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

module "test_disks" {
  source = "../../modules/detached_disks"
  
  resource_prefix = var.resource_prefix
  project_id      = var.project_id
  zone            = var.zone
  region          = var.region
  
  disks = [
    {
      name     = "zonal-disk-1"
      size     = 10
      type     = "pd-standard"
      regional = false
    },
    {
      name     = "zonal-disk-2"
      size     = 20
      type     = "pd-standard"
      regional = false
    },
    {
      name     = "regional-disk-1"
      size     = 30
      type     = "pd-standard"
      regional = true
    }
  ]
  
  labels = {
    purpose     = "integration-test"
    test_type   = "disk-migration"
    environment = "test"
  }
}