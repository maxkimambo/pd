package utils

import (
	"fmt"
	"math/rand"
	"strings"
)

func ExtractZoneName(zone string) string {

	parts := strings.Split(zone, "/")
	if len(parts) < 9 {
		return zone
	}
	return parts[len(parts)-1]
}

func ExtractDiskType(diskType string) string {

	parts := strings.Split(diskType, "/")
	if len(parts) < 9 {
		return diskType
	}
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
	if storagePoolId == "" {
		return ""
	}
	if strings.Contains(storagePoolId, "/") {
		return storagePoolId
	}
	return fmt.Sprintf("projects/%s/zones/%s/storagePools/%s", projectId, zone, storagePoolId)
}
