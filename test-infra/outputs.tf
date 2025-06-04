output "instance_details" {
  description = "Details of the created GCE instances."
  value = [
    for instance in google_compute_instance.gce_instances : {
      name              = instance.name
      zone              = instance.zone
      machine_type      = instance.machine_type
      internal_ip       = instance.network_interface[0].network_ip
      external_ip       = try(instance.network_interface[0].access_config[0].nat_ip, "N/A (no public IP assigned)")
      attached_disk_ids = [for disk in instance.attached_disk : disk.source]
    }
  ]
}

output "created_disk_details" {
  description = "Details of the created extra disks."
  value = {
    for k, disk in google_compute_disk.extra_disks : k => {
      name      = disk.name
      self_link = disk.self_link
      size_gb   = disk.size
      type      = disk.type
      zone      = disk.zone
    }
  }
}
