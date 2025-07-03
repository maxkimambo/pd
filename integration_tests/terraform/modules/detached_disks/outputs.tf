output "zonal_disk_names" {
  description = "Names of all zonal disks"
  value       = [for disk in google_compute_disk.zonal : disk.name]
}

output "zonal_disk_ids" {
  description = "IDs of all zonal disks"
  value       = [for disk in google_compute_disk.zonal : disk.id]
}

output "regional_disk_names" {
  description = "Names of all regional disks"
  value       = [for disk in google_compute_region_disk.regional : disk.name]
}

output "regional_disk_ids" {
  description = "IDs of all regional disks"
  value       = [for disk in google_compute_region_disk.regional : disk.id]
}

output "all_disk_names" {
  description = "Names of all disks (zonal and regional)"
  value = concat(
    [for disk in google_compute_disk.zonal : disk.name],
    [for disk in google_compute_region_disk.regional : disk.name]
  )
}