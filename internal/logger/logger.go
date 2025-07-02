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

var (
	// Unified logger instance
	log *UnifiedLogger
)

// init ensures logger is never nil
func init() {
	// Initialize unified logger
	log = GetLogger()
}

// CLIFormatter provides clean output for CLI applications
type CLIFormatter struct {
	DisableTimestamp bool
	DisableLevel     bool
	DisableColors    bool
}

func (f *CLIFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b bytes.Buffer

	// Simple clean format: just the message for user-facing logs
	if f.DisableLevel && f.DisableTimestamp {
		b.WriteString(entry.Message)
		b.WriteByte('\n')
		return b.Bytes(), nil
	}

	// Include level for operational logs
	if !f.DisableLevel {
		levelColor := ""
		resetColor := ""
		if !f.DisableColors {
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

	b.WriteString(entry.Message)

	// Add fields if present
	if len(entry.Data) > 0 {
		b.WriteString(" ")
		for k, v := range entry.Data {
			b.WriteString(fmt.Sprintf("%s=%v ", k, v))
		}
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

func Setup(verbose bool, jsonLogs bool, quiet bool) {
	// Check environment variables (these override CLI flags)
	if envLogMode := os.Getenv("LOG_MODE"); envLogMode != "" {
		switch envLogMode {
		case "quiet":
			quiet = true
			verbose = false
		case "verbose":
			verbose = true
			quiet = false
		case "debug":
			verbose = true
			quiet = false
		}
	}

	if envLogFormat := os.Getenv("LOG_FORMAT"); envLogFormat != "" {
		switch envLogFormat {
		case "json":
			jsonLogs = true
		case "text":
			jsonLogs = false
		}
	}

	// Get the unified logger
	ul := GetLogger()
	internalLogger := ul.GetInternalLogger()

	// Configure log level
	var level logrus.Level
	if quiet {
		level = logrus.ErrorLevel
	} else if verbose {
		level = logrus.DebugLevel
	} else {
		level = logrus.InfoLevel
	}

	// Clear any existing hooks
	internalLogger.Hooks = make(logrus.LevelHooks)

	// For JSON logs, use standard JSON formatter
	if jsonLogs {
		internalLogger.SetFormatter(&logrus.JSONFormatter{})
		internalLogger.SetLevel(level)
		internalLogger.SetOutput(io.Discard) // Output handled by hooks

		// Add routing hook for JSON
		hook := NewOutputRouterHook()
		hook.UserFormatter = &logrus.JSONFormatter{}
		hook.OpFormatter = &logrus.JSONFormatter{}
		internalLogger.AddHook(hook)
	} else {
		// For text logs, use routing hook with different formatters
		internalLogger.SetOutput(io.Discard) // Output handled by hooks
		internalLogger.SetLevel(level)
		internalLogger.SetFormatter(&logrus.TextFormatter{}) // Dummy formatter

		// Create and configure routing hook
		hook := NewOutputRouterHook()

		// User formatter - clean output
		hook.UserFormatter = &CLIFormatter{
			DisableTimestamp: true,
			DisableLevel:     true,
			DisableColors:    false,
		}

		// Op formatter - detailed output
		if verbose {
			hook.OpFormatter = &logrus.TextFormatter{
				FullTimestamp: true,
				ForceColors:   isatty.IsTerminal(os.Stderr.Fd()),
			}
		} else {
			hook.OpFormatter = &CLIFormatter{
				DisableTimestamp: true,
				DisableLevel:     false,
				DisableColors:    !isatty.IsTerminal(os.Stderr.Fd()),
			}
		}

		internalLogger.AddHook(hook)
	}
}

// Log returns the logrus standard logger (deprecated - use L() instead)
func Log() *logrus.Logger {
	return logrus.StandardLogger()
}

// L returns the unified logger instance (preferred for new code)
func L() *UnifiedLogger {
	return log
}

// Convenience methods that delegate to the unified logger

// Info logs an info message
func Info(msg string, fields ...Field) {
	log.Info(msg, fields...)
}

// Infof logs a formatted info message
func Infof(format string, args ...interface{}) {
	log.Infof(format, args...)
}

// Error logs an error message
func Error(msg string, fields ...Field) {
	log.Error(msg, fields...)
}

// Errorf logs a formatted error message
func Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

// Warn logs a warning message
func Warn(msg string, fields ...Field) {
	log.Warn(msg, fields...)
}

// Warnf logs a formatted warning message
func Warnf(format string, args ...interface{}) {
	log.Warnf(format, args...)
}

// Debug logs a debug message
func Debug(msg string, fields ...Field) {
	log.Debug(msg, fields...)
}

// Debugf logs a formatted debug message
func Debugf(format string, args ...interface{}) {
	log.Debugf(format, args...)
}

// WithField creates an entry with a single field
func WithField(key string, value interface{}) *logrus.Entry {
	return log.WithField(key, value)
}

// WithFieldsMap creates an entry with fields from a map
func WithFieldsMap(fields map[string]interface{}) *logrus.Entry {
	return log.WithFieldsMap(fields)
}

// Starting logs a start message with rocket emoji
func Starting(msg string) {
	log.Starting(msg)
}

// Success logs a success message with checkmark emoji
func Success(msg string) {
	log.Success(msg)
}

// Successf logs a formatted success message with checkmark emoji
func Successf(format string, args ...interface{}) {
	log.Successf(format, args...)
}

// Snapshot logs a snapshot message with camera emoji
func Snapshot(msg string) {
	log.Snapshot(msg)
}

// Snapshotf logs a formatted snapshot message with camera emoji
func Snapshotf(format string, args ...interface{}) {
	log.Snapshotf(format, args...)
}

// Delete logs a delete message with trash emoji
func Delete(msg string) {
	log.Delete(msg)
}

// Deletef logs a formatted delete message with trash emoji
func Deletef(format string, args ...interface{}) {
	log.Deletef(format, args...)
}

// Create logs a create message with disk emoji
func Create(msg string) {
	log.Create(msg)
}

// Createf logs a formatted create message with disk emoji
func Createf(format string, args ...interface{}) {
	log.Createf(format, args...)
}

// Cleanup logs a cleanup message with broom emoji
func Cleanup(msg string) {
	log.Cleanup(msg)
}

// Cleanupf logs a formatted cleanup message with broom emoji
func Cleanupf(format string, args ...interface{}) {
	log.Cleanupf(format, args...)
}
