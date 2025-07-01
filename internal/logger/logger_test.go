package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestLoggerInitialization(t *testing.T) {
	// Test that User and Op loggers are never nil
	if User == nil {
		t.Error("User logger should not be nil after init")
	}
	if Op == nil {
		t.Error("Op logger should not be nil after init")
	}
}

func TestUnifiedLoggerInitialization(t *testing.T) {
	// Test that GetLogger returns a non-nil logger
	ul := GetLogger()
	if ul == nil {
		t.Error("GetLogger should never return nil")
	}
	
	// Test that calling GetLogger multiple times returns the same instance
	ul2 := GetLogger()
	if ul != ul2 {
		t.Error("GetLogger should return the same instance")
	}
}

func TestLoggerSetup(t *testing.T) {
	tests := []struct {
		name     string
		verbose  bool
		jsonLogs bool
		quiet    bool
	}{
		{"Default", false, false, false},
		{"Verbose", true, false, false},
		{"Quiet", false, false, true},
		{"JSON", false, true, false},
		{"Verbose JSON", true, true, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup should not panic
			Setup(tt.verbose, tt.jsonLogs, tt.quiet)
			
			// Loggers should still be non-nil
			if User == nil {
				t.Error("User logger should not be nil after Setup")
			}
			if Op == nil {
				t.Error("Op logger should not be nil after Setup")
			}
		})
	}
}

func TestUserLoggerOutput(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer
	
	// Create a test logger
	testLogger := logrus.New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(logrus.InfoLevel)
	
	// Create a user logger with our test logger
	userLogger := &UserLogger{logger: testLogger}
	
	// Test basic logging
	userLogger.Info("test message")
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	
	// Clear buffer
	buf.Reset()
	
	// Test methods with emojis
	userLogger.Starting("starting test")
	output = buf.String()
	if !strings.Contains(output, "starting test") {
		t.Errorf("Expected output to contain 'starting test', got: %s", output)
	}
}

func TestOpLoggerOutput(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer
	
	// Create a test logger
	testLogger := logrus.New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(logrus.InfoLevel)
	
	// Create an op logger with our test logger
	opLogger := &OpLogger{logger: testLogger}
	
	// Test basic logging
	opLogger.Info("operational message")
	output := buf.String()
	if !strings.Contains(output, "operational message") {
		t.Errorf("Expected output to contain 'operational message', got: %s", output)
	}
	
	// Clear buffer
	buf.Reset()
	
	// Test with fields
	entry := opLogger.WithFields(map[string]interface{}{
		"disk": "test-disk",
		"zone": "us-central1-a",
	})
	entry.Info("disk operation")
	output = buf.String()
	if !strings.Contains(output, "disk operation") {
		t.Errorf("Expected output to contain 'disk operation', got: %s", output)
	}
}

func TestLogTypeRouting(t *testing.T) {
	// This test verifies that log_type field is properly set
	
	// Create a test hook to capture log entries
	captureHook := &testHook{entries: make([]*logrus.Entry, 0)}
	
	// Get the unified logger and add our test hook
	ul := GetLogger()
	ul.GetInternalLogger().AddHook(captureHook)
	
	// Clear existing entries
	captureHook.entries = make([]*logrus.Entry, 0)
	
	// Test User logger
	User.Info("user message")
	if len(captureHook.entries) == 0 {
		t.Fatal("Expected log entry to be captured")
	}
	
	lastEntry := captureHook.entries[len(captureHook.entries)-1]
	if logType, ok := lastEntry.Data["log_type"]; !ok || logType != string(UserLog) {
		t.Errorf("Expected log_type to be 'user', got: %v", logType)
	}
	
	// Test Op logger
	Op.Info("op message")
	if len(captureHook.entries) < 2 {
		t.Fatal("Expected second log entry to be captured")
	}
	
	lastEntry = captureHook.entries[len(captureHook.entries)-1]
	if logType, ok := lastEntry.Data["log_type"]; !ok || logType != string(OpLog) {
		t.Errorf("Expected log_type to be 'op', got: %v", logType)
	}
}

// testHook is a simple hook for capturing log entries in tests
type testHook struct {
	entries []*logrus.Entry
}

func (h *testHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *testHook) Fire(entry *logrus.Entry) error {
	h.entries = append(h.entries, entry)
	return nil
}