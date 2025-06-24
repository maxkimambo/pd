package migrator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MigrationErrorCategory categorizes errors for proper handling in worker environments
type MigrationErrorCategory string

const (
	// ErrorCategoryRetryable indicates a temporary error that can be retried
	ErrorCategoryRetryable MigrationErrorCategory = "RETRYABLE"
	
	// ErrorCategoryPermanent indicates a permanent error that should not be retried
	ErrorCategoryPermanent MigrationErrorCategory = "PERMANENT"
	
	// ErrorCategoryResourceConflict indicates a resource conflict that may resolve later
	ErrorCategoryResourceConflict MigrationErrorCategory = "RESOURCE_CONFLICT"
	
	// ErrorCategoryUserInput indicates an error due to invalid user input
	ErrorCategoryUserInput MigrationErrorCategory = "USER_INPUT"
	
	// ErrorCategoryCancelled indicates the operation was cancelled
	ErrorCategoryCancelled MigrationErrorCategory = "CANCELLED"
	
	// ErrorCategoryThrottled indicates the operation was throttled and should be retried with backoff
	ErrorCategoryThrottled MigrationErrorCategory = "THROTTLED"
)

// MigrationErrorSeverity indicates the severity level of an error
type MigrationErrorSeverity string

const (
	SeverityLow      MigrationErrorSeverity = "LOW"
	SeverityMedium   MigrationErrorSeverity = "MEDIUM"
	SeverityHigh     MigrationErrorSeverity = "HIGH"
	SeverityCritical MigrationErrorSeverity = "CRITICAL"
)

// EnhancedMigrationError provides detailed error information suitable for worker environments
type EnhancedMigrationError struct {
	// Basic error information
	Message     string
	Cause       error
	Timestamp   time.Time
	
	// Categorization for proper handling
	Category    MigrationErrorCategory
	Severity    MigrationErrorSeverity
	Phase       MigrationPhase
	
	// Context information
	JobID       string
	InstanceID  string
	ResourceID  string
	
	// Retry information
	IsRetryable bool
	RetryDelay  time.Duration
	
	// Additional context
	ErrorCode   string
	Details     map[string]interface{}
}

// Error implements the error interface
func (e *EnhancedMigrationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// IsTemporary returns true if the error is temporary and can be retried
func (e *EnhancedMigrationError) IsTemporary() bool {
	return e.Category == ErrorCategoryRetryable || e.Category == ErrorCategoryThrottled
}

// ShouldRetry returns true if the error should be retried
func (e *EnhancedMigrationError) ShouldRetry() bool {
	return e.IsRetryable
}

// GetRetryDelay returns the recommended delay before retry
func (e *EnhancedMigrationError) GetRetryDelay() time.Duration {
	return e.RetryDelay
}

// WithJobContext adds job context to the error
func (e *EnhancedMigrationError) WithJobContext(jobID, instanceID string) *EnhancedMigrationError {
	e.JobID = jobID
	e.InstanceID = instanceID
	return e
}

// WithResourceContext adds resource context to the error
func (e *EnhancedMigrationError) WithResourceContext(resourceID string) *EnhancedMigrationError {
	e.ResourceID = resourceID
	return e
}

// WithDetail adds additional detail to the error
func (e *EnhancedMigrationError) WithDetail(key string, value interface{}) *EnhancedMigrationError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// ToLogFields returns fields suitable for structured logging
func (e *EnhancedMigrationError) ToLogFields() map[string]interface{} {
	fields := map[string]interface{}{
		"error_message":   e.Message,
		"error_category":  string(e.Category),
		"error_severity":  string(e.Severity),
		"error_phase":     e.Phase,
		"error_timestamp": e.Timestamp,
		"is_retryable":    e.IsRetryable,
	}
	
	if e.JobID != "" {
		fields["job_id"] = e.JobID
	}
	if e.InstanceID != "" {
		fields["instance_id"] = e.InstanceID
	}
	if e.ResourceID != "" {
		fields["resource_id"] = e.ResourceID
	}
	if e.ErrorCode != "" {
		fields["error_code"] = e.ErrorCode
	}
	if e.RetryDelay > 0 {
		fields["retry_delay"] = e.RetryDelay
	}
	if e.Cause != nil {
		fields["underlying_error"] = e.Cause.Error()
	}
	
	// Add custom details
	for k, v := range e.Details {
		fields[fmt.Sprintf("detail_%s", k)] = v
	}
	
	return fields
}

// NewEnhancedMigrationError creates a new enhanced migration error
func NewEnhancedMigrationError(
	message string,
	category MigrationErrorCategory,
	severity MigrationErrorSeverity,
	phase MigrationPhase,
) *EnhancedMigrationError {
	return &EnhancedMigrationError{
		Message:     message,
		Category:    category,
		Severity:    severity,
		Phase:       phase,
		Timestamp:   time.Now(),
		IsRetryable: category == ErrorCategoryRetryable || category == ErrorCategoryThrottled,
		Details:     make(map[string]interface{}),
	}
}

// WrapError wraps an existing error with enhanced migration error information
func WrapError(
	err error,
	message string,
	category MigrationErrorCategory,
	severity MigrationErrorSeverity,
	phase MigrationPhase,
) *EnhancedMigrationError {
	enhanced := NewEnhancedMigrationError(message, category, severity, phase)
	enhanced.Cause = err
	return enhanced
}

// NewRetryableError creates a new retryable error with suggested delay
func NewRetryableError(
	message string,
	phase MigrationPhase,
	retryDelay time.Duration,
) *EnhancedMigrationError {
	err := NewEnhancedMigrationError(message, ErrorCategoryRetryable, SeverityMedium, phase)
	err.RetryDelay = retryDelay
	return err
}

// NewPermanentError creates a new permanent error that should not be retried
func NewPermanentError(
	message string,
	phase MigrationPhase,
	severity MigrationErrorSeverity,
) *EnhancedMigrationError {
	err := NewEnhancedMigrationError(message, ErrorCategoryPermanent, severity, phase)
	err.IsRetryable = false
	return err
}

// NewResourceConflictError creates a new resource conflict error
func NewResourceConflictError(
	message string,
	phase MigrationPhase,
	resourceID string,
) *EnhancedMigrationError {
	err := NewEnhancedMigrationError(message, ErrorCategoryResourceConflict, SeverityMedium, phase)
	err.ResourceID = resourceID
	err.RetryDelay = 30 * time.Second // Default retry delay for resource conflicts
	return err
}

// NewCancelledError creates a new cancellation error
func NewCancelledError(
	message string,
	phase MigrationPhase,
) *EnhancedMigrationError {
	err := NewEnhancedMigrationError(message, ErrorCategoryCancelled, SeverityLow, phase)
	err.IsRetryable = false
	return err
}

// CategorizeError attempts to categorize a generic error based on its content
func CategorizeError(err error, phase MigrationPhase) *EnhancedMigrationError {
	if err == nil {
		return nil
	}

	message := err.Error()
	
	// Try to categorize based on error message patterns
	switch {
	case isContextCancelledError(err):
		return WrapError(err, "Operation cancelled", ErrorCategoryCancelled, SeverityLow, phase)
	case isResourceConflictError(message):
		return WrapError(err, "Resource conflict detected", ErrorCategoryResourceConflict, SeverityMedium, phase)
	case isThrottlingError(message):
		enhanced := WrapError(err, "Operation throttled", ErrorCategoryThrottled, SeverityMedium, phase)
		enhanced.RetryDelay = 60 * time.Second
		return enhanced
	case isTemporaryError(message):
		enhanced := WrapError(err, "Temporary error", ErrorCategoryRetryable, SeverityMedium, phase)
		enhanced.RetryDelay = 30 * time.Second
		return enhanced
	case isPermissionError(message):
		return WrapError(err, "Permission denied", ErrorCategoryPermanent, SeverityHigh, phase)
	case isNotFoundError(message):
		return WrapError(err, "Resource not found", ErrorCategoryPermanent, SeverityHigh, phase)
	default:
		return WrapError(err, "Unknown error", ErrorCategoryPermanent, SeverityMedium, phase)
	}
}

// Helper functions to categorize errors based on message patterns

func isContextCancelledError(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func isResourceConflictError(message string) bool {
	conflictPatterns := []string{
		"already exists",
		"already locked",
		"in use",
		"conflict",
		"concurrent",
	}
	for _, pattern := range conflictPatterns {
		if strings.Contains(strings.ToLower(message), pattern) {
			return true
		}
	}
	return false
}

func isThrottlingError(message string) bool {
	throttlePatterns := []string{
		"rate limit",
		"throttled",
		"quota exceeded",
		"too many requests",
	}
	for _, pattern := range throttlePatterns {
		if strings.Contains(strings.ToLower(message), pattern) {
			return true
		}
	}
	return false
}

func isTemporaryError(message string) bool {
	temporaryPatterns := []string{
		"timeout",
		"temporary",
		"unavailable",
		"network",
		"connection",
		"internal error",
	}
	for _, pattern := range temporaryPatterns {
		if strings.Contains(strings.ToLower(message), pattern) {
			return true
		}
	}
	return false
}

func isPermissionError(message string) bool {
	permissionPatterns := []string{
		"permission denied",
		"unauthorized",
		"forbidden",
		"access denied",
	}
	for _, pattern := range permissionPatterns {
		if strings.Contains(strings.ToLower(message), pattern) {
			return true
		}
	}
	return false
}

func isNotFoundError(message string) bool {
	notFoundPatterns := []string{
		"not found",
		"does not exist",
		"missing",
	}
	for _, pattern := range notFoundPatterns {
		if strings.Contains(strings.ToLower(message), pattern) {
			return true
		}
	}
	return false
}