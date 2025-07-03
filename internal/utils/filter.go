package utils

import (
	"fmt"
	"strings"
)

// BuildGcpLabelFilter creates a GCP filter string from a label key=value pair
// Input examples:
//   - "env=production" → "labels.env=\"production\""
//   - "app-version=1.0 beta" → "labels.app-version=\"1.0 beta\""
//   - "my-label" → "labels.my-label:*" (presence check)
func BuildGcpLabelFilter(labelFilter string) (string, error) {
	if labelFilter == "" {
		return "", nil
	}

	// Check if it's a presence filter (key only)
	if !strings.Contains(labelFilter, "=") {
		return fmt.Sprintf("labels.%s:*", labelFilter), nil
	}

	// Split the key=value pair
	parts := strings.SplitN(labelFilter, "=", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid label filter format: %s", labelFilter)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if key == "" {
		return "", fmt.Errorf("label key cannot be empty")
	}

	// Escape any double quotes in the value
	escapedValue := strings.ReplaceAll(value, `"`, `\"`)

	// Build the filter string with proper quoting
	return fmt.Sprintf("labels.%s=\"%s\"", key, escapedValue), nil
}

// MatchesLabel checks if an instance's labels match the given filter
// This is used for client-side filtering when needed (e.g., when filtering by instance names)
func MatchesLabel(labels map[string]string, labelFilter string) (bool, error) {
	if labelFilter == "" {
		return true, nil
	}

	// Check if it's a presence filter (key only)
	if !strings.Contains(labelFilter, "=") {
		_, exists := labels[labelFilter]
		return exists, nil
	}

	// Split the key=value pair
	parts := strings.SplitN(labelFilter, "=", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid label filter format: %s", labelFilter)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if key == "" {
		return false, fmt.Errorf("label key cannot be empty")
	}

	// Check if the label exists and matches the value
	labelValue, exists := labels[key]
	if !exists {
		return false, nil
	}

	return labelValue == value, nil
}