package logging

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogging(t *testing.T) {
	t.Parallel()

	logFile := fmt.Sprintf("%s.log", t.Name())
	fileLogger, err := New(
		WithFileName(logFile),
		WithLevel("trace"),
		WithConsoleLog(false),
	)
	require.NoError(t, err, "error creating logger")
	require.NotNil(t, fileLogger, "logger should not be nil")
	require.FileExists(t, logFile, "log file should exist")
	t.Cleanup(func() {
		err := os.Remove(logFile)
		require.NoError(t, err, "error removing log file")
	})

	fileLogger.Info().Msg("This is an info log message.")
	fileLogger.Debug().Msg("This is a debug log message.")
	fileLogger.Error().Msg("This is an error log message.")
	fileLogger.Trace().Msg("This is a trace log message.")
	fileLogger.Warn().Msg("This is a warning log message.")

	logFileData, err := os.ReadFile(logFile)
	require.NoError(t, err, "error reading log file")
	require.NotEmpty(t, logFileData, "log file should not be empty")
	assert.Contains(
		t,
		string(logFileData),
		"This is an info log message.",
		"log file should contain info log message",
	)
	assert.Contains(
		t,
		string(logFileData),
		"This is a debug log message.",
		"log file should contain debug log message",
	)
	assert.Contains(
		t,
		string(logFileData),
		"This is an error log message.",
		"log file should contain error log message",
	)
	assert.Contains(
		t,
		string(logFileData),
		"This is a trace log message.",
		"log file should contain trace log message",
	)
	assert.Contains(
		t,
		string(logFileData),
		"This is a warning log message.",
		"log file should contain warning log message",
	)
}

func TestLogging_WithSoleWriter(t *testing.T) {
	t.Parallel()

	logFile := fmt.Sprintf("%s.log", t.Name())
	soleWriter := &bytes.Buffer{}
	fileLogger, err := New(
		WithFileName(logFile),
		WithLevel("trace"),
		WithConsoleLog(false),
		WithSoleWriter(soleWriter),
	)
	require.NoError(t, err, "error creating logger")
	require.NotNil(t, fileLogger, "logger should not be nil")
	t.Cleanup(func() {
		if _, err := os.Stat(logFile); err == nil {
			err := os.Remove(logFile)
			require.NoError(t, err, "error removing log file")
		}
	})

	fileLogger.Info().Msg("This is an info log message.")
	fileLogger.Debug().Msg("This is a debug log message.")
	fileLogger.Error().Msg("This is an error log message.")
	fileLogger.Trace().Msg("This is a trace log message.")
	fileLogger.Warn().Msg("This is a warning log message.")

	assert.NoFileExists(t, logFile, "log file should not exist with a sole writer specified")
	assert.Contains(
		t,
		soleWriter.String(),
		"This is an info log message.",
		"sole writer should contain info log message",
	)
	assert.Contains(
		t,
		soleWriter.String(),
		"This is a debug log message.",
		"sole writer should contain debug log message",
	)
	assert.Contains(
		t,
		soleWriter.String(),
		"This is an error log message.",
		"sole writer should contain error log message",
	)
	assert.Contains(
		t,
		soleWriter.String(),
		"This is a trace log message.",
		"sole writer should contain trace log message",
	)
	assert.Contains(
		t,
		soleWriter.String(),
		"This is a warning log message.",
		"sole writer should contain warning log message",
	)
}

func TestLogging_MustNew(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() {
		logger := MustNew()
		require.NotNil(t, logger, "logger should not be nil")
	})
}
