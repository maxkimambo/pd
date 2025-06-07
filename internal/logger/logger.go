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
	UserLog *logrus.Logger // Clean messages for users (stdout)
	OpLog   *logrus.Logger // Detailed operational logs (stderr)
)

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
	UserLog = logrus.New()
	UserLog.SetOutput(os.Stdout)
	UserLog.SetFormatter(&CLIFormatter{
		DisableTimestamp: true,
		DisableLevel:     true,
		DisableColors:    false,
	})
	
	if quiet {
		UserLog.SetLevel(logrus.ErrorLevel)
	} else {
		UserLog.SetLevel(logrus.InfoLevel)
	}
	
	// Operational logger (detailed output to stderr)
	OpLog = logrus.New()
	OpLog.SetOutput(os.Stderr)
	
	if jsonLogs {
		OpLog.SetFormatter(&logrus.JSONFormatter{})
		if verbose {
			OpLog.SetLevel(logrus.DebugLevel)
		} else {
			OpLog.SetLevel(logrus.InfoLevel)
		}
	} else {
		if verbose {
			OpLog.SetLevel(logrus.DebugLevel)
			OpLog.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
				ForceColors:   isatty.IsTerminal(os.Stderr.Fd()),
			})
		} else {
			OpLog.SetLevel(logrus.WarnLevel)
			OpLog.SetFormatter(&CLIFormatter{
				DisableTimestamp: true,
				DisableLevel:     false,
				DisableColors:    !isatty.IsTerminal(os.Stderr.Fd()),
			})
		}
	}
}

func Log() *logrus.Logger {
	return logrus.StandardLogger()
}