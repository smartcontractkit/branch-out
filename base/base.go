// Package base provides a base HTTP client with logging middleware for calling other services.
package base

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

const (
	// RateLimitWarningThreshold is the number of remaining requests before a warning is logged.
	RateLimitWarningThreshold = 5
	// RateLimitWarningMsg is the message logged when the number of remaining requests is below the warning threshold.
	RateLimitWarningMsg = "API requests nearing rate limit"
	// RateLimitHitMsg is the message logged when the number of remaining requests is 0.
	RateLimitHitMsg = "API rate limit hit, sleeping until limit reset"
)

type baseOptions struct {
	logger    zerolog.Logger
	component string
	base      http.RoundTripper
}

// Option can modify how the base client works
type Option func(*baseOptions)

// WithLogger sets the logger to use for logging requests and responses
func WithLogger(logger zerolog.Logger) Option {
	return func(opts *baseOptions) {
		opts.logger = logger
	}
}

// WithBaseTransport sets the base transport to use for the client
func WithBaseTransport(base http.RoundTripper) Option {
	return func(opts *baseOptions) {
		opts.base = base
	}
}

// WithComponent sets the component used
func WithComponent(component string) Option {
	return func(opts *baseOptions) {
		opts.component = component
	}
}

// NewClient creates a new base HTTP client with logging middleware
func NewClient(options ...Option) *http.Client {
	transport := NewTransport(options...)

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
}

// NewTransport creates a new base transport with logging middleware for use as the base transport for other clients
func NewTransport(options ...Option) http.RoundTripper {
	opts := &baseOptions{
		logger: zerolog.Nop(),
		base:   http.DefaultTransport,
	}
	for _, opt := range options {
		opt(opts)
	}

	if opts.component != "" {
		opts.logger = opts.logger.With().Str("component", opts.component).Logger()
	}

	return &LoggingTransport{
		Base:      opts.base,
		Logger:    opts.logger,
		Component: opts.component,
	}
}

// LoggingTransport is an HTTP transport that logs requests and responses
type LoggingTransport struct {
	// Base is the underlying HTTP transport
	Base http.RoundTripper
	// Logger is the logger to use for logging
	Logger zerolog.Logger
	// Component identifies the service making the request
	Component string
}

// RoundTrip implements the http.RoundTripper interface
func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	l := t.Logger.With().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Logger()
	l.Trace().Msg("HTTP client request")

	// Make the request
	resp, err := t.Base.RoundTrip(req)
	duration := time.Since(start)
	l = l.With().Str("duration", duration.String()).Logger()

	if err != nil {
		l.Error().
			Err(err).
			Msg("HTTP client error")
		return resp, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		l.Error().Err(err).Msg("Failed to read response body")
		return resp, err
	}
	resp.Body = io.NopCloser(bytes.NewBuffer(body))
	l.Trace().Int("status_code", resp.StatusCode).Str("body", string(body)).Msg("HTTP client response")

	// Process rate limit headers (GitHub style)
	if callLimitStr := resp.Header.Get("X-RateLimit-Limit"); callLimitStr != "" {
		callLimit, err := strconv.Atoi(callLimitStr)
		if err != nil {
			return resp, err
		}
		l = l.With().Int("call_limit", callLimit).Logger()
	}

	if callsUsedStr := resp.Header.Get("X-RateLimit-Used"); callsUsedStr != "" {
		callsUsed, err := strconv.Atoi(callsUsedStr)
		if err != nil {
			return resp, err
		}
		l = l.With().Int("calls_used", callsUsed).Logger()
	}

	if limitResetStr := resp.Header.Get("X-RateLimit-Reset"); limitResetStr != "" {
		limitReset, err := strconv.Atoi(limitResetStr)
		if err != nil {
			return resp, err
		}
		limitResetTime := time.Unix(int64(limitReset), 0)
		l = l.With().Time("limit_reset", limitResetTime).Logger()
	}

	if callsRemainingStr := resp.Header.Get("X-RateLimit-Remaining"); callsRemainingStr != "" {
		callsRemaining, err := strconv.Atoi(callsRemainingStr)
		if err != nil {
			return resp, err
		}
		l = l.With().Int("calls_remaining", callsRemaining).Logger()
		if callsRemaining == 0 {
			l.Warn().Msg(RateLimitHitMsg)
		} else if callsRemaining <= RateLimitWarningThreshold {
			l.Warn().Msg(RateLimitWarningMsg)
		}
	}

	l.Trace().Msg("HTTP client response")
	return resp, nil
}
