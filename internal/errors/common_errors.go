package errors

import (
	"fmt"
	"strings"
)

// Common error codes
const (
	// Disk-related error codes
	CodeDiskNotFound     = "001"
	CodeDiskAccess       = "002"
	CodeDiskAttach       = "003"
	CodeDiskDetach       = "004"
	CodeDiskMigration    = "005"
	CodeDiskSnapshot     = "006"
	
	// GCP API error codes
	CodeGCPAPI           = "001"
	CodeGCPPermissions   = "002"
	CodeGCPQuota         = "003"
	CodeGCPAuth          = "004"
	
	// Validation error codes
	CodeValidationInput  = "001"
	CodeValidationConfig = "002"
	CodeValidationZone   = "003"
	CodeValidationDisk   = "004"
	
	// Migration error codes
	CodeMigrationFailed  = "001"
	CodeMigrationTimeout = "002"
	CodeMigrationRollback = "003"
)

// NewDiskNotFoundError creates an error for when a disk cannot be found
func NewDiskNotFoundError(diskName, zone, project string) *MigrationError {
	return NewDiskError(CodeDiskNotFound, 
		fmt.Sprintf("Disk '%s' not found", diskName), 
		"Disk lookup operation").
		WithContext("disk", diskName).
		WithContext("zone", zone).
		WithContext("project", project).
		WithTroubleshooting(
			"Verify the disk name is correct and exists in the specified zone",
			"Check if you have the necessary permissions to access the disk",
			"Ensure the zone and project are correct",
			"Use 'gcloud compute disks list' to verify the disk exists",
		)
}

// NewDiskAccessError creates an error for disk access issues
func NewDiskAccessError(diskName, zone, project string, originalErr error) *MigrationError {
	return NewDiskError(CodeDiskAccess, 
		fmt.Sprintf("Failed to access disk '%s'", diskName), 
		"Disk access operation").
		WithContext("disk", diskName).
		WithContext("zone", zone).
		WithContext("project", project).
		WithOriginalError(originalErr).
		WithTroubleshooting(
			"Verify you have the necessary IAM permissions for the disk",
			"Check if the disk is in use by another instance",
			"Ensure the service account has Compute Engine permissions",
			"Try running 'gcloud auth list' to verify authentication",
		)
}

// NewZoneValidationError creates an error for invalid zone parameters
func NewZoneValidationError(zone, operation string) *MigrationError {
	return NewValidationError(CodeValidationZone,
		fmt.Sprintf("Invalid zone '%s'", zone),
		operation).
		WithContext("zone", zone).
		WithTroubleshooting(
			"Use a valid GCP zone (e.g., 'us-central1-a', 'europe-west1-b')",
			"Run 'gcloud compute zones list' to see available zones",
			"Ensure the zone exists in your project's region",
			"Check that the zone supports the resources you're trying to create",
		)
}

// NewGCPAPIError creates an error for GCP API failures
func NewGCPAPIError(operation string, originalErr error) *MigrationError {
	errMsg := "GCP API request failed"
	if originalErr != nil {
		errMsg = fmt.Sprintf("GCP API request failed: %v", originalErr)
	}
	
	err := NewGCPError(CodeGCPAPI, errMsg, operation).
		WithOriginalError(originalErr)
	
	// Add specific troubleshooting based on error message
	if originalErr != nil {
		errStr := strings.ToLower(originalErr.Error())
		if strings.Contains(errStr, "permission") || strings.Contains(errStr, "unauthorized") {
			err = err.WithTroubleshooting(
				"Check your GCP authentication: 'gcloud auth list'",
				"Verify service account has necessary permissions",
				"Ensure you're authenticated for the correct project",
				"Check IAM roles include required Compute Engine permissions",
			)
		} else if strings.Contains(errStr, "quota") || strings.Contains(errStr, "exceeded") {
			err = err.WithTroubleshooting(
				"Check your GCP quotas in the Console",
				"Request quota increase if needed",
				"Try the operation in a different region/zone",
				"Consider reducing the number of concurrent operations",
			)
		} else if strings.Contains(errStr, "not found") {
			err = err.WithTroubleshooting(
				"Verify the resource name and location are correct",
				"Check if the resource exists in the specified project",
				"Confirm you're operating in the correct GCP project",
				"Use gcloud commands to verify resource existence",
			)
		} else {
			err = err.WithTroubleshooting(
				"Check your internet connection and GCP service status",
				"Verify your GCP credentials are valid and not expired",
				"Try the operation again after a brief delay",
				"Check GCP Console for any service disruptions",
			)
		}
	}
	
	return err
}

// NewMigrationFailedError creates an error for migration failures
func NewMigrationFailedError(instanceName, diskName string, originalErr error) *MigrationError {
	return NewMigrationError(ErrorCategoryMigration, CodeMigrationFailed,
		fmt.Sprintf("Migration failed for disk '%s' on instance '%s'", diskName, instanceName),
		"Disk migration operation").
		WithContext("instance", instanceName).
		WithContext("disk", diskName).
		WithOriginalError(originalErr).
		WithTroubleshooting(
			"Check the migration logs for detailed error information",
			"Verify the target disk type is supported by the instance",
			"Ensure sufficient quota and permissions for the migration",
			"Consider running the migration with lower concurrency",
			"Check if the disk is attached and the instance is running",
		)
}

// NewValidationFailedError creates an error for input validation failures
func NewValidationFailedError(field, value, operation string) *MigrationError {
	return NewValidationError(CodeValidationInput,
		fmt.Sprintf("Invalid value for %s: '%s'", field, value),
		operation).
		WithContext("field", field).
		WithContext("value", value).
		WithTroubleshooting(
			"Check the command syntax and parameter values",
			"Refer to the tool documentation for valid parameter formats",
			"Use --help to see available options and examples",
			"Verify resource names follow GCP naming conventions",
		)
}

// IsRetryableError determines if an error is retryable
func IsRetryableError(err error) bool {
	if migErr, ok := err.(*MigrationError); ok {
		// Network errors and some GCP API errors are retryable
		if migErr.Category == ErrorCategoryNetwork {
			return true
		}
		if migErr.Category == ErrorCategoryGCP {
			// Specific GCP errors that are retryable
			if migErr.OriginalError != nil {
				errStr := strings.ToLower(migErr.OriginalError.Error())
				return strings.Contains(errStr, "timeout") ||
					   strings.Contains(errStr, "unavailable") ||
					   strings.Contains(errStr, "internal error") ||
					   strings.Contains(errStr, "503") ||
					   strings.Contains(errStr, "502")
			}
		}
	}
	return false
}

// GetErrorSeverity returns the severity level of an error
func GetErrorSeverity(err error) string {
	if migErr, ok := err.(*MigrationError); ok {
		switch migErr.Category {
		case ErrorCategoryValidation, ErrorCategoryConfiguration:
			return "WARNING"
		case ErrorCategoryPermission, ErrorCategoryGCP:
			return "ERROR"
		case ErrorCategoryMigration, ErrorCategoryDisk:
			return "CRITICAL"
		default:
			return "ERROR"
		}
	}
	return "ERROR"
}