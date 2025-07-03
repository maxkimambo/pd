variable "resource_prefix" {
  description = "A unique prefix to ensure resource names are isolated per test run"
  type        = string
}

variable "project_id" {
  description = "The Google Cloud project ID"
  type        = string
}

variable "zone" {
  description = "The Google Cloud zone"
  type        = string
}

variable "machine_type" {
  description = "Machine type for the test instance"
  type        = string
  default     = "c3-standard-4"
}

variable "disk_type" {
  description = "Source disk type for migration test"
  type        = string
  default     = "pd-balanced"
}

variable "boot_disk_type" {
  description = "Boot disk type for the instance"
  type        = string
  default     = "pd-balanced"
}