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
  default     = ""
}

variable "region" {
  description = "The Google Cloud region for regional disks"
  type        = string
  default     = ""
}

variable "disks" {
  description = "List of disk configurations"
  type = list(object({
    name       = string
    size       = number
    type       = string
    regional   = bool
  }))
}

variable "labels" {
  description = "Labels to apply to all disks"
  type        = map(string)
  default = {
    purpose = "integration-test"
  }
}