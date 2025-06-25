package errors

import (
	"fmt"
	"strings"
)

// ErrorCategory represents the category of error
type ErrorCategory string

const (
	// ErrorCategoryGCP represents Google Cloud API errors
	ErrorCategoryGCP ErrorCategory = "GCP"
	// ErrorCategoryValidation represents validation errors
	ErrorCategoryValidation ErrorCategory = "VALIDATION"
	// ErrorCategoryPermission represents permission/authorization errors
	ErrorCategoryPermission ErrorCategory = "PERMISSION"
	// ErrorCategoryNetwork represents network connectivity errors
	ErrorCategoryNetwork ErrorCategory = "NETWORK"
	// ErrorCategoryConfiguration represents configuration errors
	ErrorCategoryConfiguration ErrorCategory = "CONFIGURATION"
	// ErrorCategoryMigration represents migration-specific errors
	ErrorCategoryMigration ErrorCategory = "MIGRATION"
	// ErrorCategoryDisk represents disk operation errors
	ErrorCategoryDisk ErrorCategory = "DISK"
	// ErrorCategoryInstance represents instance operation errors
	ErrorCategoryInstance ErrorCategory = "INSTANCE"
)

// MigrationError represents a structured error with context and troubleshooting information
type MigrationError struct {
	Category         ErrorCategory
	Code             string
	Message          string
	Operation        string
	Context          map[string]interface{}
	Troubleshooting  []string
	OriginalError    error
}

// Error implements the error interface
func (e *MigrationError) Error() string {
	var sb strings.Builder
	
	// Error header with category and code
	sb.WriteString(fmt.Sprintf("%s-%s: %s", e.Category, e.Code, e.Message))
	
	// Add operation context if available
	if e.Operation != "" {
		sb.WriteString(fmt.Sprintf("\nOperation: %s", e.Operation))
	}
	
	// Add relevant context
	if len(e.Context) > 0 {
		sb.WriteString("\nContext:")
		for key, value := range e.Context {
			sb.WriteString(fmt.Sprintf("\n  %s: %v", key, value))
		}
	}
	
	// Add troubleshooting steps
	if len(e.Troubleshooting) > 0 {
		sb.WriteString("\nTroubleshooting:")
		for i, step := range e.Troubleshooting {
			sb.WriteString(fmt.Sprintf("\n  %d. %s", i+1, step))
		}
	}
	
	// Add original error if available
	if e.OriginalError != nil {
		sb.WriteString(fmt.Sprintf("\nUnderlying error: %v", e.OriginalError))
	}
	
	return sb.String()
}

// Unwrap returns the original error for error chain compatibility
func (e *MigrationError) Unwrap() error {
	return e.OriginalError
}

// NewMigrationError creates a new migration error with the specified parameters
func NewMigrationError(category ErrorCategory, code, message, operation string) *MigrationError {
	return &MigrationError{
		Category:        category,
		Code:            code,
		Message:         message,
		Operation:       operation,
		Context:         make(map[string]interface{}),
		Troubleshooting: []string{},
	}
}

// WithContext adds context information to the error
func (e *MigrationError) WithContext(key string, value interface{}) *MigrationError {
	e.Context[key] = value
	return e
}

// WithTroubleshooting adds troubleshooting steps to the error
func (e *MigrationError) WithTroubleshooting(steps ...string) *MigrationError {
	e.Troubleshooting = append(e.Troubleshooting, steps...)
	return e
}

// WithOriginalError adds the original error to the migration error
func (e *MigrationError) WithOriginalError(err error) *MigrationError {
	e.OriginalError = err
	return e
}

// Common error constructors

// NewDiskError creates a new disk-related error
func NewDiskError(code, message, operation string) *MigrationError {
	return NewMigrationError(ErrorCategoryDisk, code, message, operation)
}

// NewGCPError creates a new GCP API error
func NewGCPError(code, message, operation string) *MigrationError {
	return NewMigrationError(ErrorCategoryGCP, code, message, operation)
}

// NewValidationError creates a new validation error
func NewValidationError(code, message, operation string) *MigrationError {
	return NewMigrationError(ErrorCategoryValidation, code, message, operation)
}

// NewPermissionError creates a new permission error
func NewPermissionError(code, message, operation string) *MigrationError {
	return NewMigrationError(ErrorCategoryPermission, code, message, operation)
}

// NewConfigurationError creates a new configuration error
func NewConfigurationError(code, message, operation string) *MigrationError {
	return NewMigrationError(ErrorCategoryConfiguration, code, message, operation)
}