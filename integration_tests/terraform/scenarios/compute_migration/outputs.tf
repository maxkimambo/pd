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