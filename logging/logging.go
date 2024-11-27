/*
Copyright (C) 2024 Steve Miller KC1AWV

This program is free software: you can redistribute it and/or modify it
under the terms of the GNU General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option)
any later version.

This program is distributed in the hope that it will be useful, but WITHOUT
ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
more details.

You should have received a copy of the GNU General Public License along with
this program. If not, see <http://www.gnu.org/licenses/>.
*/

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
