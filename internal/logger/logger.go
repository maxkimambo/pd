package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Setup configures the global logrus logger based on the debug flag.
func Setup(debug bool) {
	logrus.SetOutput(os.Stdout) // Ensure logs go to stdout

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true, // Or check terminal capabilities
		})
		logrus.Debug("Debug logging enabled")
	} else {
		logrus.SetLevel(logrus.InfoLevel)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true, // Or check terminal capabilities
			// DisableLevelTruncation: true, // Optional: prevent INFO from being truncated
		})
	}
}

// Log returns the configured global logrus logger instance.
// This isn't strictly necessary as logrus functions (Info, Debug, Error) are global,
// but can be useful if you want to pass the logger instance around.
func Log() *logrus.Logger {
	return logrus.StandardLogger()
}
