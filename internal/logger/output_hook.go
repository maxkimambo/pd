package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// OutputRouterHook routes log entries to different outputs based on log_type
type OutputRouterHook struct {
	UserFormatter logrus.Formatter
	OpFormatter   logrus.Formatter
	UserWriter    io.Writer
	OpWriter      io.Writer
}

// NewOutputRouterHook creates a new output router hook
func NewOutputRouterHook() *OutputRouterHook {
	return &OutputRouterHook{
		UserFormatter: &CLIFormatter{
			DisableTimestamp: true,
			DisableLevel:     true,
			DisableColors:    false,
		},
		OpFormatter: &CLIFormatter{
			DisableTimestamp: false,
			DisableLevel:     false,
			DisableColors:    false,
		},
		UserWriter: os.Stdout,
		OpWriter:   os.Stderr,
	}
}

// Levels returns all log levels (this hook processes all levels)
func (h *OutputRouterHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire is called when a log event is fired
func (h *OutputRouterHook) Fire(entry *logrus.Entry) error {
	// Check log type
	logType, _ := entry.Data["log_type"].(string)
	
	var formatter logrus.Formatter
	var writer io.Writer
	
	if logType == string(UserLog) {
		formatter = h.UserFormatter
		writer = h.UserWriter
		
		// Add emoji to message if present
		if emoji, ok := entry.Data["emoji"].(string); ok && emoji != "" {
			entry.Message = emoji + " " + entry.Message
		}
	} else {
		formatter = h.OpFormatter
		writer = h.OpWriter
	}
	
	// Format the entry
	bytes, err := formatter.Format(entry)
	if err != nil {
		return err
	}
	
	// Write to appropriate output
	_, err = writer.Write(bytes)
	return err
}