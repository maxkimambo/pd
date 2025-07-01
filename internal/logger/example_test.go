package logger_test

import (
	"github.com/maxkimambo/pd/internal/logger"
)

func Example_unifiedLogger() {
	// Get the unified logger instance
	log := logger.GetLogger()
	
	// Basic logging
	log.Info("Starting application")
	log.Error("An error occurred")
	
	// User-facing logs with emojis
	log.Starting("Starting disk migration")
	log.Success("Migration completed")
	log.Snapshot("Creating snapshot")
	
	// Operational logs with fields
	log.WithFieldsMap(map[string]interface{}{
		"disk": "my-disk",
		"zone": "us-central1-a",
	}).Info("Disk operation started")
	
	// Using structured fields
	fields := []logger.Field{
		logger.WithLogType(logger.UserLog),
		logger.WithEmoji("💾"),
	}
	fields = append(fields, logger.WithFields(map[string]interface{}{
		"disk_name": "data-disk-1",
		"size_gb": 100,
	})...)
	log.Info("Processing disk", fields...)
}

func Example_compatibility() {
	// The existing API still works
	logger.User.Info("User message")
	logger.User.Starting("Starting process")
	logger.User.Success("Process completed")
	
	logger.Op.Info("Operational message")
	logger.Op.WithFields(map[string]interface{}{
		"component": "disk-migrator",
		"action": "snapshot",
	}).Debug("Creating snapshot")
}