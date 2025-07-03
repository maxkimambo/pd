output "zonal_disk_names" {
  description = "Names of all zonal disks"
  value       = module.test_disks.zonal_disk_names
}

output "regional_disk_names" {
  description = "Names of all regional disks"
  value       = module.test_disks.regional_disk_names
}

output "all_disk_names" {
  description = "Names of all disks"
  value       = module.test_disks.all_disk_names
}