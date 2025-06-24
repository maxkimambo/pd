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
	suffix := fmt.Sprintf("%x", rand.Intn(0xFFF))
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
