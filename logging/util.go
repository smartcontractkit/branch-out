package logging

import (
	"os"

	"github.com/rs/zerolog"
)

const (
	logLevelEnvVar = "BRANCH_OUT_LOG_LEVEL"
)

// GetLogLevel returns the log level based on the input string or environment variable.
// If the input string is empty, it checks the environment variable.
// If both are empty or invalid, it defaults to InfoLevel.
func getLogLevel(logLevelInput string) zerolog.Level {
	level, err := zerolog.ParseLevel(logLevelInput)
	if err == nil {
		return level
	}

	envLogLevel := os.Getenv(logLevelEnvVar)
	if envLogLevel != "" {
		level, err := zerolog.ParseLevel(logLevelInput)
		if err == nil {
			return level
		}
	}

	return zerolog.InfoLevel
}
