package errors

import (
	"fmt"
	"strings"
)

// DisplayError formats an error for user-friendly display
func DisplayError(err error) string {
	if migErr, ok := err.(*MigrationError); ok {
		return migErr.Error()
	}
	
	// For non-migration errors, provide a simple format
	return fmt.Sprintf("Error: %v", err)
}

// DisplayErrorSummary provides a brief summary of the error for logs
func DisplayErrorSummary(err error) string {
	if migErr, ok := err.(*MigrationError); ok {
		return fmt.Sprintf("%s-%s: %s", migErr.Category, migErr.Code, migErr.Message)
	}
	
	errStr := err.Error()
	if len(errStr) > 100 {
		return errStr[:97] + "..."
	}
	return errStr
}

// ShouldDisplayTroubleshooting determines if troubleshooting info should be shown
func ShouldDisplayTroubleshooting(err error) bool {
	if migErr, ok := err.(*MigrationError); ok {
		return len(migErr.Troubleshooting) > 0
	}
	return false
}

// FormatForCLI formats an error for command-line display with proper spacing
func FormatForCLI(err error) string {
	if migErr, ok := err.(*MigrationError); ok {
		var sb strings.Builder
		
		// Error header
		sb.WriteString(fmt.Sprintf("\n%s Error [%s-%s]\n", 
			string(migErr.Category), migErr.Category, migErr.Code))
		sb.WriteString(fmt.Sprintf("  %s\n", migErr.Message))
		
		// Operation context
		if migErr.Operation != "" {
			sb.WriteString(fmt.Sprintf("\nFailed Operation: %s\n", migErr.Operation))
		}
		
		// Context details
		if len(migErr.Context) > 0 {
			sb.WriteString("\nDetails:\n")
			for key, value := range migErr.Context {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
			}
		}
		
		// Troubleshooting steps
		if len(migErr.Troubleshooting) > 0 {
			sb.WriteString("\nHow to resolve:\n")
			for i, step := range migErr.Troubleshooting {
				sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step))
			}
		}
		
		// Original error (for debugging)
		if migErr.OriginalError != nil {
			sb.WriteString(fmt.Sprintf("\nTechnical details: %v\n", migErr.OriginalError))
		}
		
		return sb.String()
	}
	
	return fmt.Sprintf("\nError: %v\n", err)
}

// IsUserError determines if an error is due to user input/configuration
func IsUserError(err error) bool {
	if migErr, ok := err.(*MigrationError); ok {
		return migErr.Category == ErrorCategoryValidation ||
			   migErr.Category == ErrorCategoryConfiguration
	}
	return false
}

// GetErrorCode extracts the error code for reporting
func GetErrorCode(err error) string {
	if migErr, ok := err.(*MigrationError); ok {
		return fmt.Sprintf("%s-%s", migErr.Category, migErr.Code)
	}
	return "UNKNOWN"
}