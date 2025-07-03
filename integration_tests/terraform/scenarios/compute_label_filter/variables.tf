variable "resource_prefix" {
  description = "Prefix for all resources"
  type        = string
}

variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "zone" {
  description = "GCP Zone"
  type        = string
}

variable "machine_type" {
  description = "Machine type for the instance"
  type        = string
  default     = "n2-standard-2"
}

variable "disk_type" {
  description = "Type of persistent disk to create"
  type        = string
  default     = "pd-standard"
}

variable "boot_disk_type" {
  description = "Type of boot disk"
  type        = string
  default     = "pd-standard"
}

variable "random_data_size_mb" {
  description = "Size of random data to generate on each disk in MB"
  type        = number
  default     = 100
}

variable "create_test_instance" {
  description = "Whether to create the test instance"
  type        = bool
  default     = true
}

variable "test_instance_labels" {
  description = "Labels to apply to the target test instance"
  type        = map(string)
  default     = {}
}