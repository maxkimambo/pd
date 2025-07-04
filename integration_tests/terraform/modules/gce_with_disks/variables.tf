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

variable "instance_name" {
  description = "Name of the compute instance"
  type        = string
}

variable "machine_type" {
  description = "Machine type for the instance"
  type        = string
  default     = "c3-standard-4"
}

variable "attached_disks" {
  description = "List of disk configurations to attach to the instance"
  type = list(object({
    name = string
    size = number
    type = string
  }))
  default = []
}

variable "boot_disk_size" {
  description = "Size of the boot disk in GB"
  type        = number
  default     = 10
}

variable "boot_disk_type" {
  description = "Type of the boot disk"
  type        = string
  default     = "pd-balanced"
}

variable "labels" {
  description = "Labels to apply to all resources"
  type        = map(string)
  default = {
    purpose = "integration-test"
  }
}

variable "random_data_size_mb" {
  description = "Size of test data to generate on each disk in MB using fio (0 to disable)"
  type        = number
  default     = 0
}