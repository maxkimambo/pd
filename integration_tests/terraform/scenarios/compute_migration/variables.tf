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
  default     = "e2-micro"
}