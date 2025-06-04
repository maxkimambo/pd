variable "instance_count" {
  description = "Number of GCE instances to create."
  type        = number
  default     = 1
}

variable "disk_count_per_instance" {
  description = "Number of extra disks to attach to each instance."
  type        = number
  default     = 2
}

variable "project_id" {
  description = "The GCP project ID."
  type        = string
 
}

variable "zone" {
  description = "The GCP zone to deploy resources in."
  type        = string
  default     = "us-central1-a"
}

variable "machine_type" {
  description = "The machine type for the GCE instances."
  type        = string
  default     = "e2-medium"
}

variable "boot_disk_image" {
  description = "The image to use for the boot disk."
  type        = string
  default     = "ubuntu-os-cloud/ubuntu-2204-lts" 
}

variable "disk_size_gb" {
  description = "Size of each extra disk in GB."
  type        = number
  default     = 10
}

variable "disk_type" {
  description = "Type of the extra disks. Must be 'pd-ssd' as per requirement."
  type        = string
  default     = "pd-ssd"
  validation {
    condition     = var.disk_type == "pd-ssd"
    error_message = "The disk_type must be 'pd-ssd'."
  }
}

variable "instance_name_prefix" {
  description = "Prefix for the GCE instance names."
  type        = string
  default     = "gce-vm"
}

variable "disk_name_prefix" {
  description = "Prefix for the extra disk names."
  type        = string
  default     = "extra-disk"
}
