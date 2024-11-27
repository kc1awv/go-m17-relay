package logging

import (
	"github.com/sirupsen/logrus"
)

var logger = logrus.New()

func InitLogLevel(level string) {
	switch level {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}
}

func LogDebug(message string, fields map[string]interface{}) {
	entry := logger.WithFields(fields)
	entry.Debug(message)
}

func LogInfo(message string, fields map[string]interface{}) {
	entry := logger.WithFields(fields)
	entry.Info(message)
}

func LogWarn(message string, fields map[string]interface{}) {
	entry := logger.WithFields(fields)
	entry.Warn(message)
}

func LogError(message string, fields map[string]interface{}) {
	entry := logger.WithFields(fields)
	entry.Error(message)
}
