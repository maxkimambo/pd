package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

func Setup(debug bool) {
	logrus.SetOutput(os.Stdout) 

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true, 
		})
		logrus.Debug("Debug logging enabled")
	} else {
		logrus.SetLevel(logrus.InfoLevel)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true, 
		})
	}
}

func Log() *logrus.Logger {
	return logrus.StandardLogger()
}
