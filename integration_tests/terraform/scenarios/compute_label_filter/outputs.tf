output "target_instance_name" {
  value = module.target_instance.instance_name
}

output "decoy_instance_name" {
  value = module.decoy_instance.instance_name
}

output "target_instance_labels" {
  value = module.target_instance.instance_labels
}

output "decoy_instance_labels" {
  value = module.decoy_instance.instance_labels
}

output "target_attached_disk_names" {
  value = module.target_instance.attached_disk_names
}

output "decoy_attached_disk_names" {
  value = module.decoy_instance.attached_disk_names
}