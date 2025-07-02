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