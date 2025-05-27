package utils

import "strings"

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
