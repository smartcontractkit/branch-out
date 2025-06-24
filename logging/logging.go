// Package logging provides a configurable logger for the server.
package logging

import (
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// TimeLayout is the default time layout for the logger.
const TimeLayout = "2006-01-02T15:04:05.000Z"

var once sync.Once

// options holds the options for the logger.
type options struct {
	disableConsoleLog  bool
	logLevelInput      string
	logFileName        string
	disableFileLogging bool
	writers            []io.Writer
}

// Option is a function that sets an option for the logger.
type Option func(*options)

// WithWriters sets the writers to use for logging.
// This is useful for testing logging output.
func WithWriters(writers ...io.Writer) Option {
	return func(o *options) {
		o.writers = writers
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

// DisableConsoleLog disables console logging.
func DisableConsoleLog() Option {
	return func(o *options) {
		o.disableConsoleLog = true
	}
}

// DisableFileLogging disables only logging to a file.
func DisableFileLogging() Option {
	return func(o *options) {
		o.disableFileLogging = true
	}
}

func defaultOptions() *options {
	return &options{
		disableConsoleLog: false,
		logLevelInput:     "info",
		logFileName:       "branch-out.log.json",
	}
}

// New initializes a new logger with the specified options.
func New(options ...Option) (zerolog.Logger, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	var (
		logFileName        = opts.logFileName
		logLevelInput      = opts.logLevelInput
		disableConsoleLog  = opts.disableConsoleLog
		disableFileLogging = opts.disableFileLogging
	)

	writers := opts.writers
	if !disableFileLogging {
		err := os.WriteFile(logFileName, []byte{}, 0600)
		if err != nil {
			return zerolog.Logger{}, err
		}
		lumberLogger := &lumberjack.Logger{
			Filename:   logFileName,
			MaxSize:    50, // megabytes
			MaxBackups: 10,
			MaxAge:     30,
		}
		writers = append(writers, lumberLogger)
	}
	if !disableConsoleLog {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: TimeLayout})
	}

	logLevel, err := zerolog.ParseLevel(logLevelInput)
	if err != nil {
		return zerolog.Logger{}, err
	}

	once.Do(func() {
		zerolog.TimeFieldFormat = TimeLayout
	})
	multiWriter := zerolog.MultiLevelWriter(writers...)
	logger := zerolog.New(multiWriter).Level(logLevel).With().Timestamp().Logger()
	return logger, nil
}
