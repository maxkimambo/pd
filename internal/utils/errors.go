package utils

import (
	"fmt"
	"strings"
)

// ErrorContext provides additional context for error messages
type ErrorContext struct {
	Operation   string
	Resource    string
	Reason      string
	Suggestion  string
	Command     string
}

// FormatError creates a detailed error message with context
func FormatError(ctx ErrorContext, err error) string {
	var sb strings.Builder
	
	sb.WriteString("\n[ERROR] ")
	if ctx.Operation != "" {
		sb.WriteString(fmt.Sprintf("Failed to %s", ctx.Operation))
		if ctx.Resource != "" {
			sb.WriteString(fmt.Sprintf(" for '%s'", ctx.Resource))
		}
	} else {
		sb.WriteString("Operation failed")
	}
	sb.WriteString("\n")
	
	if ctx.Reason != "" {
		sb.WriteString(fmt.Sprintf("        Reason: %s\n", ctx.Reason))
	} else if err != nil {
		sb.WriteString(fmt.Sprintf("        Reason: %v\n", err))
	}
	
	if ctx.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("        Action: %s\n", ctx.Suggestion))
	}
	
	if ctx.Command != "" {
		sb.WriteString(fmt.Sprintf("        Command to check: %s\n", ctx.Command))
	}
	
	return sb.String()
}

// QuotaExceededError formats a quota exceeded error with helpful suggestions
func QuotaExceededError(resource, location string) string {
	return FormatError(ErrorContext{
		Operation:  fmt.Sprintf("allocate %s", resource),
		Resource:   location,
		Reason:     "Insufficient quota",
		Suggestion: "Increase quota or free up existing resources",
		Command:    "gcloud compute project-info describe --project=PROJECT_ID",
	}, nil)
}

// PermissionError formats a permission error with helpful suggestions
func PermissionError(operation, resource string) string {
	return FormatError(ErrorContext{
		Operation:  operation,
		Resource:   resource,
		Reason:     "Insufficient permissions",
		Suggestion: "Ensure your service account has the required IAM roles",
		Command:    "gcloud projects get-iam-policy PROJECT_ID",
	}, nil)
}

// ResourceNotFoundError formats a resource not found error
func ResourceNotFoundError(resourceType, resourceName string) string {
	return FormatError(ErrorContext{
		Operation:  fmt.Sprintf("find %s", resourceType),
		Resource:   resourceName,
		Reason:     fmt.Sprintf("%s does not exist", resourceType),
		Suggestion: fmt.Sprintf("Verify the %s name and location", resourceType),
		Command:    fmt.Sprintf("gcloud compute %ss list", strings.ToLower(resourceType)),
	}, nil)
}