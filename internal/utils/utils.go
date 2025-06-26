package utils

import (
	"fmt"
	"math/rand"
	"strings"
)

func ExtractZoneName(zone string) string {
	if zone == "" {
		return ""
	}
	parts := strings.Split(zone, "/")
	return parts[len(parts)-1]
}

// ExtractZoneFromDiskURL extracts the zone name from a GCP disk URL
// URL format: projects/PROJECT/zones/ZONE/disks/DISK_NAME
func ExtractZoneFromDiskURL(diskURL string) string {
	if diskURL == "" {
		return ""
	}
	parts := strings.Split(diskURL, "/")
	// Look for the "zones" part and get the next element
	for i, part := range parts {
		if part == "zones" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func ExtractDiskType(diskType string) string {
	if diskType == "" {
		return ""
	}
	parts := strings.Split(diskType, "/")
	return parts[len(parts)-1]
}

func ExtractMachineType(machineType string) string {
	if machineType == "" {
		return ""
	}
	parts := strings.Split(machineType, "/")
	return parts[len(parts)-1]
}

func AddSuffix(name string, length int) string {
	if length == 0 {
		length = 4
	}
	// Generate a random hex string with the specified length
	maxVal := 1
	for i := 0; i < length; i++ {
		maxVal *= 16
	}
	suffix := fmt.Sprintf("%0*x", length, rand.Intn(maxVal))
	return fmt.Sprintf("%s-%s", name, suffix)
}

func GetStoragePoolURL(projectId, storagePoolId, zone string) string {
	if projectId == "" || zone == "" || storagePoolId == "" {
		return ""
	}

	return fmt.Sprintf("projects/%s/zones/%s/storagePools/%s", projectId, zone, storagePoolId)
}

func GetDiskUrl(projectId, zone, diskName string) string {
	if projectId == "" || zone == "" || diskName == "" {
		return ""
	}

	return fmt.Sprintf("projects/%s/zones/%s/disks/%s", projectId, zone, diskName)
}


func ParseLabels(labels string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(labels, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		result[key] = value
	}
	return result
}