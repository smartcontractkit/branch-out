// Package logging provides a configurable logger for the server.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// TimeLayout is the default time layout for the logger.
const TimeLayout = "2006-01-02T15:04:05.000Z"

var once sync.Once

// options holds the options for the logger.
type options struct {
	enableConsoleLog bool
	logLevelInput    string
	logFileName      string
	writers          []io.Writer
	soleWriter       io.Writer
	secrets          []string
}

// Option is a function that sets an option for the logger.
type Option func(*options)

// WithWriters sets adds additional writers to use for logging.
// This is useful for testing logging output.
func WithWriters(writers ...io.Writer) Option {
	return func(o *options) {
		o.writers = append(o.writers, writers...)
	}
}

// WithSoleWriter sets the sole writer to use for logging.
// This is useful for testing logging output.
func WithSoleWriter(writer io.Writer) Option {
	return func(o *options) {
		o.soleWriter = writer
	}
}

// WithFileName sets the log file name.
func WithFileName(logFileName string) Option {
	return func(o *options) {
		o.logFileName = logFileName
	}
}

// WithLevel sets the log level.
func WithLevel(logLevelInput string) Option {
	return func(o *options) {
		o.logLevelInput = logLevelInput
	}
}

// WithConsoleLog enables or disables console logging.
func WithConsoleLog(enabled bool) Option {
	return func(o *options) {
		o.enableConsoleLog = enabled
	}
}

// WithSecrets sets the secrets to redact in the logs.
func WithSecrets(secrets []string) Option {
	return func(o *options) {
		o.secrets = secrets
	}
}

func defaultOptions() *options {
	return &options{
		enableConsoleLog: true,
	}
}

// Return a writer that writes to the specified file and redacts sensitive information.
func withRedactWriter(writer io.Writer, secrets []string) io.Writer {
	if len(secrets) == 0 {
		return writer
	}
	return &redactWriter{
		Writer:  writer,
		Secrets: secrets,
	}
}

// New initializes a new logger with the specified options.
func New(options ...Option) (zerolog.Logger, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	var (
		logFileName      = opts.logFileName
		logLevelInput    = opts.logLevelInput
		enableConsoleLog = opts.enableConsoleLog
	)

	writers := opts.writers
	if opts.soleWriter != nil {
		writers = []io.Writer{withRedactWriter(opts.soleWriter, opts.secrets)}
	} else {
		if logFileName != "" {
			err := os.MkdirAll(filepath.Dir(logFileName), 0700)
			if err != nil {
				return zerolog.Logger{}, err
			}
			err = os.WriteFile(logFileName, []byte{}, 0600)
			if err != nil {
				return zerolog.Logger{}, err
			}
			lumberLogger := &lumberjack.Logger{
				Filename:   logFileName,
				MaxSize:    50, // megabytes
				MaxBackups: 10,
				MaxAge:     30,
			}
			writers = append(writers, withRedactWriter(lumberLogger, opts.secrets))
		}
		if enableConsoleLog {
			writers = append(writers, withRedactWriter(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: TimeLayout}, opts.secrets))
		}
	}
	logLevel, err := zerolog.ParseLevel(logLevelInput)
	if err != nil {
		return zerolog.Logger{}, fmt.Errorf("invalid log level: %w", err)
	}

	once.Do(func() {
		zerolog.TimeFieldFormat = TimeLayout
	})
	multiWriter := zerolog.MultiLevelWriter(writers...)
	logger := zerolog.New(multiWriter).Level(logLevel).With().Timestamp().Logger()
	return logger, nil
}

// MustNew initializes a new logger with the specified options.
// It panics if there is an error.
func MustNew(options ...Option) zerolog.Logger {
	logger, err := New(options...)
	if err != nil {
		panic(err)
	}
	return logger
}
