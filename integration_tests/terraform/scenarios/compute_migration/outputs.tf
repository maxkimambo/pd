output "instance_name" {
  description = "The name of the test instance"
  value       = module.test_instance.instance_name
}

output "instance_zone" {
  description = "The zone of the test instance"
  value       = module.test_instance.instance_zone
}

output "attached_disk_names" {
  description = "Names of all attached disks"
  value       = module.test_instance.attached_disk_names
}

output "disk_mount_paths" {
  description = "Mount paths for attached disks"
  value       = module.test_instance.disk_mount_paths
}

output "random_data_size_mb" {
  description = "Size of test data generated on each disk"
  value       = module.test_instance.random_data_size_mb
}