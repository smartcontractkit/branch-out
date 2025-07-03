// Package client provides a Resty client with logging middleware to call other services with.
package client

import (
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog"
)

// NewResty creates a new Resty client with logging middleware to call other services with.
// Component is used to identify what service is making the request.
func NewResty(logger zerolog.Logger, component string) *resty.Client {
	client := resty.New()
	logger = logger.With().Str("component", component).Logger()

	client.OnBeforeRequest(func(_ *resty.Client, r *resty.Request) error {
		logger.Trace().
			Str("method", r.Method).
			Str("url", r.URL).
			Msg("HTTP client request")
		return nil
	})

	client.OnAfterResponse(func(_ *resty.Client, r *resty.Response) error {
		logger.Trace().
			Str("method", r.Request.Method).
			Str("url", r.Request.URL).
			Int("status_code", r.StatusCode()).
			Str("duration", r.Time().String()).
			Bytes("response_body", r.Body()).
			Msg("HTTP client response")
		return nil
	})

	client.OnError(func(r *resty.Request, err error) {
		logger.Error().
			Err(err).
			Str("method", r.Method).
			Str("url", r.URL).
			Msg("HTTP client error")
	})

	return client
}
