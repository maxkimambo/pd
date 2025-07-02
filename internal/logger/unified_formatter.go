package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
)

// UnifiedFormatter formats logs based on the log_type field
type UnifiedFormatter struct {
	// For user logs
	UserOutput io.Writer
	// For operational logs
	OpOutput io.Writer
	// Whether to show colors
	EnableColors bool
	// Whether to show timestamp for op logs
	ShowTimestamp bool
	// Whether to show log level for op logs
	ShowLevel bool
	// Whether to output JSON
	JSONFormat bool
}

// NewUnifiedFormatter creates a formatter with sensible defaults
func NewUnifiedFormatter() *UnifiedFormatter {
	return &UnifiedFormatter{
		UserOutput:    os.Stdout,
		OpOutput:      os.Stderr,
		EnableColors:  isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stderr.Fd()),
		ShowTimestamp: false,
		ShowLevel:     true,
		JSONFormat:    false,
	}
}

// Format implements logrus.Formatter
func (f *UnifiedFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Check log type from fields
	logType, _ := entry.Data["log_type"].(string)
	emoji, _ := entry.Data["emoji"].(string)

	// For JSON format, use standard JSON formatter
	if f.JSONFormat {
		jsonFormatter := &logrus.JSONFormatter{}
		return jsonFormatter.Format(entry)
	}

	// Route to appropriate formatter based on log type
	if logType == string(UserLog) {
		return f.formatUserLog(entry, emoji)
	}
	return f.formatOpLog(entry)
}

// formatUserLog formats clean user-facing messages
func (f *UnifiedFormatter) formatUserLog(entry *logrus.Entry, emoji string) ([]byte, error) {
	var b bytes.Buffer

	// Add emoji if present
	if emoji != "" {
		b.WriteString(emoji)
		b.WriteString(" ")
	}

	// Add message
	b.WriteString(entry.Message)
	b.WriteByte('\n')

	return b.Bytes(), nil
}

// formatOpLog formats operational logs with more detail
func (f *UnifiedFormatter) formatOpLog(entry *logrus.Entry) ([]byte, error) {
	var b bytes.Buffer

	// Add timestamp if enabled
	if f.ShowTimestamp {
		b.WriteString(entry.Time.Format("2006-01-02 15:04:05"))
		b.WriteString(" ")
	}

	// Add level if enabled
	if f.ShowLevel {
		levelColor := ""
		resetColor := ""
		if f.EnableColors {
			switch entry.Level {
			case logrus.ErrorLevel:
				levelColor = "\033[31m" // Red
			case logrus.WarnLevel:
				levelColor = "\033[33m" // Yellow
			case logrus.InfoLevel:
				levelColor = "\033[36m" // Cyan
			case logrus.DebugLevel:
				levelColor = "\033[37m" // White
			}
			resetColor = "\033[0m"
		}

		b.WriteString(levelColor)
		b.WriteString(strings.ToUpper(entry.Level.String()))
		b.WriteString(resetColor)
		b.WriteString(": ")
	}

	// Add message
	b.WriteString(entry.Message)

	// Add fields (excluding internal ones)
	if len(entry.Data) > 0 {
		b.WriteString(" ")
		first := true
		for k, v := range entry.Data {
			// Skip internal fields
			if k == "log_type" || k == "emoji" {
				continue
			}
			if !first {
				b.WriteString(" ")
			}
			b.WriteString(fmt.Sprintf("%s=%v", k, v))
			first = false
		}
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

// SetupUnifiedLogger configures the unified logger with appropriate settings
func SetupUnifiedLogger(verbose bool, jsonLogs bool, quiet bool) {
	ul := GetLogger()

	formatter := NewUnifiedFormatter()
	formatter.JSONFormat = jsonLogs

	// Configure based on flags
	var level logrus.Level
	if quiet {
		level = logrus.ErrorLevel
	} else if verbose {
		level = logrus.DebugLevel
		formatter.ShowTimestamp = true
	} else {
		level = logrus.InfoLevel
	}

	// Create a multi-writer that routes based on log type
	multiWriter := &LogTypeWriter{
		UserWriter: formatter.UserOutput,
		OpWriter:   formatter.OpOutput,
	}

	ul.Configure(multiWriter, level, formatter)
}

// LogTypeWriter routes output based on log_type field
type LogTypeWriter struct {
	UserWriter io.Writer
	OpWriter   io.Writer
}

// Write implements io.Writer but this is a hack - the formatter should handle routing
func (w *LogTypeWriter) Write(p []byte) (n int, err error) {
	// This is called after formatting, so we can't inspect the log type here
	// Instead, we'll default to stdout for now and handle routing in a custom hook
	return w.UserWriter.Write(p)
}
