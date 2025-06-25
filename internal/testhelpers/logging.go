// Package testhelpers provides utilities for testing.
package testhelpers

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/logging"
)

// Logger returns a zerolog.Logger for the test.
// It creates a log file in the current directory with the test name.
// It also cleans up the log file after the test.
// If the test fails, it leaves the log file for debugging.
func Logger(tb testing.TB, options ...logging.Option) zerolog.Logger {
	tb.Helper()

	logFile := fmt.Sprintf("%s.log.json", tb.Name())
	logFile = strings.ReplaceAll(logFile, "/", "_")

	defaultOptions := []logging.Option{
		logging.WithFileName(logFile),
		logging.WithLevel("trace"),
		logging.WithConsoleLog(false),
	}

	logger, err := logging.New(
		append(defaultOptions, options...)...,
	)
	require.NoError(tb, err)
	logger = logger.With().Str("test_name", tb.Name()).Str("log_file", logFile).Logger()
	tb.Cleanup(func() {
		if tb.Failed() {
			logger.Error().Msg("Test failed, leaving log file for debugging")
		} else {
			if err := os.Remove(logFile); err != nil {
				logger.Error().Err(err).Msg("Error removing log file")
			}
		}
	})
	return logger
}
