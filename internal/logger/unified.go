package logger

import (
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// LogType represents the type of log message
type LogType string

const (
	UserLog LogType = "user"
	OpLog   LogType = "op"
)

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
	Value interface{}
}

// UnifiedLogger is the main logger interface
type UnifiedLogger struct {
	mu     sync.RWMutex
	logger *logrus.Logger
}

var (
	// unifiedLog is the global logger instance
	unifiedLog *UnifiedLogger
	once       sync.Once
)

// GetLogger returns the global logger instance, initializing it if necessary
func GetLogger() *UnifiedLogger {
	once.Do(func() {
		initDefaultLogger()
	})
	return unifiedLog
}

// initDefaultLogger creates a default logger configuration
func initDefaultLogger() {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&CLIFormatter{
		DisableTimestamp: true,
		DisableLevel:     true,
		DisableColors:    false,
	})

	unifiedLog = &UnifiedLogger{
		logger: logger,
	}
}

// WithLogType creates a field for the log type
func WithLogType(logType LogType) Field {
	return Field{Key: "log_type", Value: string(logType)}
}

// WithEmoji creates a field for emoji
func WithEmoji(emoji string) Field {
	return Field{Key: "emoji", Value: emoji}
}

// WithFields creates fields from a map
func WithFields(fields map[string]interface{}) []Field {
	result := make([]Field, 0, len(fields))
	for k, v := range fields {
		result = append(result, Field{Key: k, Value: v})
	}
	return result
}

// entry creates a logrus.Entry with the given fields
func (l *UnifiedLogger) entry(fields ...Field) *logrus.Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	logFields := make(logrus.Fields)
	for _, field := range fields {
		logFields[field.Key] = field.Value
	}

	return l.logger.WithFields(logFields)
}

// Info logs an info message
func (l *UnifiedLogger) Info(msg string, fields ...Field) {
	l.entry(fields...).Info(msg)
}

// Infof logs a formatted info message
func (l *UnifiedLogger) Infof(format string, args ...interface{}) {
	l.entry().Infof(format, args...)
}

// Error logs an error message
func (l *UnifiedLogger) Error(msg string, fields ...Field) {
	l.entry(fields...).Error(msg)
}

// Errorf logs a formatted error message
func (l *UnifiedLogger) Errorf(format string, args ...interface{}) {
	l.entry().Errorf(format, args...)
}

// Warn logs a warning message
func (l *UnifiedLogger) Warn(msg string, fields ...Field) {
	l.entry(fields...).Warn(msg)
}

// Warnf logs a formatted warning message
func (l *UnifiedLogger) Warnf(format string, args ...interface{}) {
	l.entry().Warnf(format, args...)
}

// Debug logs a debug message
func (l *UnifiedLogger) Debug(msg string, fields ...Field) {
	l.entry(fields...).Debug(msg)
}

// Debugf logs a formatted debug message
func (l *UnifiedLogger) Debugf(format string, args ...interface{}) {
	l.entry().Debugf(format, args...)
}

// WithField creates an entry with a single field
func (l *UnifiedLogger) WithField(key string, value interface{}) *logrus.Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.WithField(key, value)
}

// WithFieldsMap creates an entry with fields from a map
func (l *UnifiedLogger) WithFieldsMap(fields map[string]interface{}) *logrus.Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.WithFields(fields)
}

// Configure updates the logger configuration
func (l *UnifiedLogger) Configure(output io.Writer, level logrus.Level, formatter logrus.Formatter) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.logger.SetOutput(output)
	l.logger.SetLevel(level)
	l.logger.SetFormatter(formatter)
}

// GetInternalLogger returns the underlying logrus logger (use with caution)
func (l *UnifiedLogger) GetInternalLogger() *logrus.Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger
}

// Convenience methods for user-facing logs with emojis

// Starting logs a start message with status prefix
func (l *UnifiedLogger) Starting(msg string) {
	l.Info("[STARTING] " + msg, WithLogType(UserLog))
}

// Success logs a success message with status prefix
func (l *UnifiedLogger) Success(msg string) {
	l.Info("[SUCCESS] " + msg, WithLogType(UserLog))
}

// Successf logs a formatted success message with status prefix
func (l *UnifiedLogger) Successf(format string, args ...interface{}) {
	l.entry(WithLogType(UserLog)).Infof("[SUCCESS] " + format, args...)
}

// Snapshot logs a snapshot message with status prefix
func (l *UnifiedLogger) Snapshot(msg string) {
	l.Info("[SNAPSHOT] " + msg, WithLogType(UserLog))
}

// Snapshotf logs a formatted snapshot message with status prefix
func (l *UnifiedLogger) Snapshotf(format string, args ...interface{}) {
	l.entry(WithLogType(UserLog)).Infof("[SNAPSHOT] " + format, args...)
}

// Delete logs a delete message with status prefix
func (l *UnifiedLogger) Delete(msg string) {
	l.Info("[DELETE] " + msg, WithLogType(UserLog))
}

// Deletef logs a formatted delete message with status prefix
func (l *UnifiedLogger) Deletef(format string, args ...interface{}) {
	l.entry(WithLogType(UserLog)).Infof("[DELETE] " + format, args...)
}

// Create logs a create message with status prefix
func (l *UnifiedLogger) Create(msg string) {
	l.Info("[CREATE] " + msg, WithLogType(UserLog))
}

// Createf logs a formatted create message with status prefix
func (l *UnifiedLogger) Createf(format string, args ...interface{}) {
	l.entry(WithLogType(UserLog)).Infof("[CREATE] " + format, args...)
}

// Cleanup logs a cleanup message with status prefix
func (l *UnifiedLogger) Cleanup(msg string) {
	l.Info("[CLEANUP] " + msg, WithLogType(UserLog))
}

// Cleanupf logs a formatted cleanup message with status prefix
func (l *UnifiedLogger) Cleanupf(format string, args ...interface{}) {
	l.entry(WithLogType(UserLog)).Infof("[CLEANUP] " + format, args...)
}
