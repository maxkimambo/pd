package logger

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
)

var (
	User *UserLogger // Clean messages for users (stdout) with emojis
	Op   *OpLogger   // Detailed operational logs (stderr) without emojis
)

type UserLogger struct {
	logger *logrus.Logger
}

type OpLogger struct {
	logger *logrus.Logger
}

// UserLogger methods with emojis built-in
func (u *UserLogger) Info(msg string) {
	u.logger.Info("🚀 " + msg)
}

func (u *UserLogger) Infof(format string, args ...interface{}) {
	u.logger.Infof("🚀 "+format, args...)
}

func (u *UserLogger) Error(msg string) {
	u.logger.Error("❌ " + msg)
}

func (u *UserLogger) Errorf(format string, args ...interface{}) {
	u.logger.Errorf("❌ "+format, args...)
}

func (u *UserLogger) Warn(msg string) {
	u.logger.Warn("⚠️ " + msg)
}

func (u *UserLogger) Warnf(format string, args ...interface{}) {
	u.logger.Warnf("⚠️ "+format, args...)
}

// Specific operation methods with relevant emojis
func (u *UserLogger) Starting(msg string) {
	u.logger.Info("🚀 " + msg)
}

func (u *UserLogger) Success(msg string) {
	u.logger.Info("✅ " + msg)
}

func (u *UserLogger) Successf(format string, args ...interface{}) {
	u.logger.Infof("✅ "+format, args...)
}

func (u *UserLogger) Snapshot(msg string) {
	u.logger.Info("📸 " + msg)
}

func (u *UserLogger) Snapshotf(format string, args ...interface{}) {
	u.logger.Infof("📸 "+format, args...)
}

func (u *UserLogger) Delete(msg string) {
	u.logger.Info("🗑️ " + msg)
}

func (u *UserLogger) Deletef(format string, args ...interface{}) {
	u.logger.Infof("🗑️ "+format, args...)
}

func (u *UserLogger) Create(msg string) {
	u.logger.Info("💾 " + msg)
}

func (u *UserLogger) Createf(format string, args ...interface{}) {
	u.logger.Infof("💾 "+format, args...)
}

func (u *UserLogger) Cleanup(msg string) {
	u.logger.Info("🧹 " + msg)
}

func (u *UserLogger) Cleanupf(format string, args ...interface{}) {
	u.logger.Infof("🧹 "+format, args...)
}

// OpLogger methods without emojis - clean operational logs
func (o *OpLogger) Info(msg string) {
	o.logger.Info(msg)
}

func (o *OpLogger) Infof(format string, args ...interface{}) {
	o.logger.Infof(format, args...)
}

func (o *OpLogger) Error(msg string) {
	o.logger.Error(msg)
}

func (o *OpLogger) Errorf(format string, args ...interface{}) {
	o.logger.Errorf(format, args...)
}

func (o *OpLogger) Warn(msg string) {
	o.logger.Warn(msg)
}

func (o *OpLogger) Warnf(format string, args ...interface{}) {
	o.logger.Warnf(format, args...)
}

func (o *OpLogger) Debug(msg string) {
	o.logger.Debug(msg)
}

func (o *OpLogger) Debugf(format string, args ...interface{}) {
	o.logger.Debugf(format, args...)
}

func (o *OpLogger) WithFields(fields map[string]interface{}) *logrus.Entry {
	return o.logger.WithFields(fields)
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
	// User-facing logger (clean output to stdout)
	userLog := logrus.New()
	userLog.SetOutput(os.Stdout)
	userLog.SetFormatter(&CLIFormatter{
		DisableTimestamp: true,
		DisableLevel:     true,
		DisableColors:    false,
	})

	if quiet {
		userLog.SetLevel(logrus.ErrorLevel)
	} else {
		userLog.SetLevel(logrus.InfoLevel)
	}

	User = &UserLogger{logger: userLog}

	// Operational logger (detailed output to stderr)
	opLog := logrus.New()
	opLog.SetOutput(os.Stderr)

	if jsonLogs {
		opLog.SetFormatter(&logrus.JSONFormatter{})
		if verbose {
			opLog.SetLevel(logrus.DebugLevel)
		} else {
			opLog.SetLevel(logrus.InfoLevel)
		}
	} else {
		if verbose {
			opLog.SetLevel(logrus.DebugLevel)
			opLog.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
				ForceColors:   isatty.IsTerminal(os.Stderr.Fd()),
			})
		} else {
			opLog.SetLevel(logrus.WarnLevel)
			opLog.SetFormatter(&CLIFormatter{
				DisableTimestamp: true,
				DisableLevel:     false,
				DisableColors:    !isatty.IsTerminal(os.Stderr.Fd()),
			})
		}
	}

	Op = &OpLogger{logger: opLog}
}

func Log() *logrus.Logger {
	return logrus.StandardLogger()
}
