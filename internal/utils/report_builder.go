package utils

import (
	"fmt"
	"strings"
)

// ReportBuilder provides a fluent interface for building formatted reports
type ReportBuilder struct {
	lines      []string
	separator  string
	width      int
}

// NewReportBuilder creates a new report builder
func NewReportBuilder() *ReportBuilder {
	return &ReportBuilder{
		lines:     []string{},
		separator: "=",
		width:     40,
	}
}

// WithSeparator sets the separator character
func (rb *ReportBuilder) WithSeparator(sep string) *ReportBuilder {
	rb.separator = sep
	return rb
}

// WithWidth sets the separator width
func (rb *ReportBuilder) WithWidth(width int) *ReportBuilder {
	rb.width = width
	return rb
}

// Header adds a header with separator
func (rb *ReportBuilder) Header(text string) *ReportBuilder {
	rb.lines = append(rb.lines, text)
	rb.lines = append(rb.lines, strings.Repeat(rb.separator, rb.width))
	return rb
}

// Section adds a section header
func (rb *ReportBuilder) Section(title string) *ReportBuilder {
	rb.lines = append(rb.lines, fmt.Sprintf("\n%s", title))
	return rb
}

// AddLine adds a single line
func (rb *ReportBuilder) AddLine(text string) *ReportBuilder {
	rb.lines = append(rb.lines, text)
	return rb
}

// AddBullet adds a bulleted line
func (rb *ReportBuilder) AddBullet(text string) *ReportBuilder {
	rb.lines = append(rb.lines, fmt.Sprintf("• %s", text))
	return rb
}

// AddNumbered adds a numbered line
func (rb *ReportBuilder) AddNumbered(number int, text string) *ReportBuilder {
	rb.lines = append(rb.lines, fmt.Sprintf("%d. %s", number, text))
	return rb
}

// AddKeyValue adds a key-value pair
func (rb *ReportBuilder) AddKeyValue(key, value string) *ReportBuilder {
	rb.lines = append(rb.lines, fmt.Sprintf("%s: %s", key, value))
	return rb
}

// AddIndented adds an indented line
func (rb *ReportBuilder) AddIndented(text string, level int) *ReportBuilder {
	indent := strings.Repeat("  ", level)
	rb.lines = append(rb.lines, fmt.Sprintf("%s%s", indent, text))
	return rb
}

// AddSeparator adds a separator line
func (rb *ReportBuilder) AddSeparator() *ReportBuilder {
	rb.lines = append(rb.lines, strings.Repeat(rb.separator, rb.width))
	return rb
}

// AddEmptyLine adds an empty line
func (rb *ReportBuilder) AddEmptyLine() *ReportBuilder {
	rb.lines = append(rb.lines, "")
	return rb
}

// Build returns the built report as a string
func (rb *ReportBuilder) Build() string {
	return strings.Join(rb.lines, "\n")
}

// BuildLines returns the built report as a slice of lines
func (rb *ReportBuilder) BuildLines() []string {
	return rb.lines
}