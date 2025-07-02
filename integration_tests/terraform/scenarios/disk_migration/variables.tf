variable "resource_prefix" {
  description = "A unique prefix to ensure resource names are isolated per test run"
  type        = string
}

variable "project_id" {
  description = "The Google Cloud project ID"
  type        = string
}

variable "zone" {
  description = "The Google Cloud zone for zonal disks"
  type        = string
}

variable "region" {
  description = "The Google Cloud region for regional disks"
  type        = string
}