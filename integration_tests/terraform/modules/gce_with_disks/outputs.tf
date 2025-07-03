output "instance_name" {
  description = "The name of the created instance"
  value       = google_compute_instance.default.name
}

output "instance_id" {
  description = "The ID of the created instance"
  value       = google_compute_instance.default.id
}

output "instance_zone" {
  description = "The zone of the created instance"
  value       = google_compute_instance.default.zone
}

output "attached_disk_names" {
  description = "Names of all attached disks"
  value       = [for disk in google_compute_disk.attached : disk.name]
}

output "attached_disk_ids" {
  description = "IDs of all attached disks"
  value       = [for disk in google_compute_disk.attached : disk.id]
}

output "disk_mount_paths" {
  description = "Mount paths for attached disks"
  value       = [for disk in var.attached_disks : "/mnt/${disk.name}"]
}

output "random_data_size_mb" {
  description = "Size of test data generated on each disk in MB using fio"
  value       = var.random_data_size_mb
}

output "instance_labels" {
  description = "Labels applied to the instance"
  value       = google_compute_instance.default.labels
}