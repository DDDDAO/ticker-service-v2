package logger

import (
	"os"

	"github.com/ddddao/ticker-service-v2/internal/config"
	"github.com/sirupsen/logrus"
)

var (
	// loggingConfig stores the current logging configuration
	loggingConfig config.LoggingConfig
)

// Init initializes the logger with the given configuration
func Init(cfg config.LoggingConfig) {
	// Store the config for later use
	loggingConfig = cfg
	
	// In production mode, only show errors and warnings
	var level logrus.Level
	if cfg.Production {
		level = logrus.WarnLevel
	} else {
		var err error
		level, err = logrus.ParseLevel(cfg.Level)
		if err != nil {
			logrus.Warnf("Invalid log level %s, using info", cfg.Level)
			level = logrus.InfoLevel
		}
	}
	logrus.SetLevel(level)

	// Set log format
	if cfg.Format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "time",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
		// Add app field for Google Cloud Logging
		logrus.WithField("app", "ticker-service-v2")
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			FullTimestamp:   true,
		})
	}

	logrus.SetOutput(os.Stdout)
}

// WithExchange returns a logger with exchange field
func WithExchange(exchange string) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"exchange": exchange,
		"app":      "ticker-service-v2",
	})
}

// WithSymbol returns a logger with exchange and symbol fields
func WithSymbol(exchange, symbol string) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"exchange": exchange,
		"symbol":   symbol,
		"app":      "ticker-service-v2",
	})
}

// ShouldLogTickerUpdates returns true if ticker updates should be logged
func ShouldLogTickerUpdates() bool {
	return loggingConfig.TickerUpdates
}

// IsProduction returns true if running in production mode
func IsProduction() bool {
	return loggingConfig.Production
}