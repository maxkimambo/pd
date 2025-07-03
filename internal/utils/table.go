package utils

import (
	"fmt"
	"strings"
)

// TableFormatter helps create formatted tables for CLI output
type TableFormatter struct {
	headers []string
	rows    [][]string
	widths  []int
}

// NewTableFormatter creates a new table formatter with headers
func NewTableFormatter(headers []string) *TableFormatter {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	return &TableFormatter{
		headers: headers,
		rows:    [][]string{},
		widths:  widths,
	}
}

// AddRow adds a row to the table
func (t *TableFormatter) AddRow(row []string) {
	if len(row) != len(t.headers) {
		return
	}
	t.rows = append(t.rows, row)
	// Update column widths
	for i, cell := range row {
		if len(cell) > t.widths[i] {
			t.widths[i] = len(cell)
		}
	}
}

// String returns the formatted table
func (t *TableFormatter) String() string {
	var sb strings.Builder

	// Top border
	t.writeBorder(&sb, "┌", "┬", "┐")

	// Headers
	sb.WriteString("│")
	for i, h := range t.headers {
		sb.WriteString(fmt.Sprintf(" %-*s ", t.widths[i], h))
		sb.WriteString("│")
	}
	sb.WriteString("\n")

	// Header separator
	t.writeBorder(&sb, "├", "┼", "┤")

	// Rows
	for _, row := range t.rows {
		sb.WriteString("│")
		for i, cell := range row {
			sb.WriteString(fmt.Sprintf(" %-*s ", t.widths[i], cell))
			sb.WriteString("│")
		}
		sb.WriteString("\n")
	}

	// Bottom border
	t.writeBorder(&sb, "└", "┴", "┘")

	return sb.String()
}

func (t *TableFormatter) writeBorder(sb *strings.Builder, left, middle, right string) {
	sb.WriteString(left)
	for i, w := range t.widths {
		sb.WriteString(strings.Repeat("─", w+2))
		if i < len(t.widths)-1 {
			sb.WriteString(middle)
		}
	}
	sb.WriteString(right)
	sb.WriteString("\n")
}