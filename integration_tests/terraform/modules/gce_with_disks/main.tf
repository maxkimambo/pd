terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

resource "google_compute_disk" "attached" {
  count   = length(var.attached_disks)
  project = var.project_id
  zone    = var.zone
  name    = "${var.resource_prefix}-${var.attached_disks[count.index].name}"
  type    = var.attached_disks[count.index].type
  size    = var.attached_disks[count.index].size
  labels  = merge(var.labels, { resource_prefix = var.resource_prefix })
}

resource "google_compute_instance" "default" {
  project      = var.project_id
  zone         = var.zone
  name         = "${var.resource_prefix}-${var.instance_name}"
  machine_type = var.machine_type
  
  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
      size  = var.boot_disk_size
      type  = var.boot_disk_type
    }
  }
  
  dynamic "attached_disk" {
    for_each = google_compute_disk.attached
    content {
      source      = attached_disk.value.id
      device_name = attached_disk.value.name
    }
  }
  
  network_interface {
    network = "default"
    access_config {}
  }
  
  labels = merge(var.labels, { resource_prefix = var.resource_prefix })
  
  metadata = {
    startup-script = <<-EOF
      #!/bin/bash
      set -e
      
      echo "Instance started at $(date)" > /tmp/startup-time.txt
      
      # Wait for disks to be attached and format them
      MAX_RETRIES=12
      RETRY_INTERVAL=10
      
      # Format and mount attached disks
      disk_index=0
      %{ for disk in var.attached_disks ~}
      echo "Setting up disk: ${disk.name}"
      
      device_path=""
      for ((i=1; i<=MAX_RETRIES; i++)); do
        device=$(ls -la /dev/disk/by-id/ | grep "google-${var.resource_prefix}-${disk.name}" | grep -v part | awk '{print $NF}' | head -1)
        if [ -n "$device" ]; then
          device_path=$(readlink -f "/dev/disk/by-id/$device")
          echo "Found device: $device_path for disk ${disk.name}"
          break
        fi
        echo "Waiting for device for disk ${disk.name} to appear... (attempt $i/$MAX_RETRIES)"
        sleep $RETRY_INTERVAL
      done

      if [ -n "$device_path" ]; then
        # Format the disk
        sudo mkfs.ext4 -F "$device_path"
        
        # Create mount point
        sudo mkdir -p "/mnt/${disk.name}"
        
        # Mount the disk
        sudo mount "$device_path" "/mnt/${disk.name}"
        
        # Add to fstab for persistent mounting
        echo "$device_path /mnt/${disk.name} ext4 defaults,nofail 0 2" | sudo tee -a /etc/fstab
        
        # Set permissions
        sudo chmod 755 "/mnt/${disk.name}"
        
        %{ if var.random_data_size_mb > 0 ~}
        # Generate test data on the disk using fio
        echo "Generating ${var.random_data_size_mb}MB of test data on /mnt/${disk.name} using fio"
        
        # Install fio if not already installed
        if ! command -v fio &> /dev/null; then
          echo "Installing fio..."
          sudo apt-get update && sudo apt-get install -y fio
        fi
        
        # Generate random data with fio for realistic testing
        sudo fio --name=stress-test --size=${var.random_data_size_mb}M \
          --filename="/mnt/${disk.name}/test_data.dat" \
          --ioengine=libaio --rw=randwrite --bs=4k \
          --direct=1 --numjobs=1 --random_distribution=random \
          --buffer_compress_percentage=0 --refill_buffers \
          --randrepeat=0 --minimal
        
        echo "Data generation complete for ${disk.name}"
        %{ endif ~}
        
        echo "Disk ${disk.name} setup complete"
      else
        echo "WARNING: Could not find device for disk ${disk.name}"
      fi
      
      disk_index=$((disk_index + 1))
      %{ endfor ~}
      
      echo "All disk setup operations completed at $(date)" >> /tmp/startup-time.txt
    EOF
  }
}