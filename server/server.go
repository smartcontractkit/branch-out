// Package server hosts the HTTP server for the branch-out application.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/logging"
	"github.com/smartcontractkit/branch-out/trunk"
)

// Server is the HTTP server for the branch-out application.
type Server struct {
	logger     zerolog.Logger
	webhookURL string
	port       int
	server     *http.Server
}

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(data)
	rw.size += size
	return size, err
}

type options struct {
	logger     zerolog.Logger
	webhookURL string
	port       int
}

// Option is a functional option that configures the server.
// Default options are used if no options are provided.
type Option func(*options)

// WithLogger sets the logger for the server.
func WithLogger(logger zerolog.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

// WithWebhookURL sets the webhook URL for the server.
func WithWebhookURL(webhookURL string) Option {
	return func(opts *options) {
		opts.webhookURL = webhookURL
	}
}

// WithPort sets the port for the server.
func WithPort(port int) Option {
	return func(opts *options) {
		opts.port = port
	}
}

// defaultOptions returns the default options for the server.
func defaultOptions() *options {
	return &options{
		logger:     logging.MustNew(),
		webhookURL: "",
		port:       0, // 0 means to use a random free port
	}
}

// New creates a new Server.
func New(options ...Option) *Server {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	return &Server{
		logger:     opts.logger,
		webhookURL: opts.webhookURL,
		port:       opts.port,
	}
}

// loggingMiddleware logs all incoming HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the response writer to capture status code and response size
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default status code
		}

		// Call the next handler
		next.ServeHTTP(rw, r)

		// Log the request
		duration := time.Since(start)

		s.logger.Trace().
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Int("status_code", rw.statusCode).
			Int("response_size", rw.size).
			Dur("duration", duration).
			Str("protocol", r.Proto).
			Msg("Processed request")
	})
}

// Start starts the server and blocks until shutdown.
// It handles both programmatic shutdown (via context) and OS signals.
func (s *Server) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.logger.Info().Str("webhookURL", s.webhookURL).Int("port", s.port).Msg("Starting server")

	baseMux := http.NewServeMux()
	baseMux.HandleFunc("/", indexHandler(s))
	baseMux.HandleFunc("/health", healthHandler(s))

	webhookMux := http.NewServeMux()
	webhookMux.HandleFunc("/", webhookIndexHandler(s))
	webhookMux.HandleFunc("/trunk", webhookHandler(s))
	baseMux.Handle("/webhooks/", webhookMux)

	// Wrap the mux with logging middleware
	handler := s.loggingMiddleware(baseMux)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: handler,
	}

	// Create a channel to listen for OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		s.logger.Info().Msg("Server listening for requests")
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrChan <- err
		}
	}()

	// Wait for shutdown signal
	select {
	case err := <-serverErrChan:
		s.logger.Error().Err(err).Msg("Server error")
		return err
	case sig := <-sigChan:
		s.logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	case <-ctx.Done():
		s.logger.Info().Msg("Context cancelled, shutting down")
	}

	return s.shutdown()
}

// WaitHealthy blocks until the server is healthy or the context is done.
func (s *Server) WaitHealthy(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			health, err := s.Health()
			if err != nil {
				return err
			}
			if health.Status == "healthy" {
				return nil
			}
		}
	}
}

// shutdown gracefully shuts down the server.
func (s *Server) shutdown() error {
	s.logger.Info().Msg("Initiating graceful shutdown")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		s.logger.Error().Err(err).Msg("Graceful shutdown failed, forcing close")
		// Force close the server if graceful shutdown fails
		if closeErr := s.server.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("Failed to force close server")
			return closeErr
		}
		return err
	}

	s.logger.Info().Msg("Server shutdown complete")
	return nil
}

// Stop programmatically stops the server (for testing or programmatic control).
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}
	return s.shutdown()
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// Health checks the health of the server and returns health status.
func (s *Server) Health() (HealthResponse, error) {
	s.logger.Debug().Msg("Health check requested")

	// You can add additional health checks here
	// For example: database connectivity, external service status, etc.

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
	}

	return response, nil
}

// WebhookEndpoint is the target endpoint that a webhook is sent to.
type WebhookEndpoint string

const (
	// WebhookEndpointTrunk is the target endpoint for Trunk webhooks.
	WebhookEndpointTrunk WebhookEndpoint = "trunk"
)

// WebhookResponse represents the response from webhook processing
type WebhookResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   error  `json:"error,omitempty"`
}

// WebhookRequest represents the structure of incoming webhook data
type WebhookRequest struct {
	Endpoint WebhookEndpoint   `json:"-"` // Path of the webhook endpoint
	Payload  json.RawMessage   `json:"-"` // Raw payload for flexible handling
	Headers  map[string]string `json:"-"` // Request headers
}

// ReceiveWebhook processes webhook data and returns the result.
func (s *Server) ReceiveWebhook(req *WebhookRequest) (*WebhookResponse, error) {
	l := s.logger.With().
		Str("endpoint", string(req.Endpoint)).
		Int("payload_size_bytes", len(req.Payload)).
		Logger()
	l.Debug().Msg("Processing webhook call")

	switch req.Endpoint {
	case WebhookEndpointTrunk:
		return nil, trunk.ReceiveWebhook(l, req.Payload)
	default:
		return nil, fmt.Errorf("unknown webhook endpoint: %s", req.Endpoint)
	}
}

// HTTP Handlers - These are thin wrappers around the core methods

func indexHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		info := map[string]any{
			"service":     "Branch-Out",
			"description": "Trunk.io Integration Service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(info); err != nil {
			s.logger.Error().Err(err).Msg("Failed to encode index response")
		}
	}
}

func healthHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		l := s.logger.With().Str("handler", "health").Logger()
		health, err := s.Health()
		if err != nil {
			l.Error().Err(err).Msg("Health check failed")
			http.Error(w, "Health check failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(health); err != nil {
			l.Error().Err(err).Msg("Failed to encode health response")
		}
	}
}

func webhookIndexHandler(s *Server) http.HandlerFunc {
	return indexHandler(s)
}

func webhookHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := s.logger.With().Str("handler", "trunk.webhook").Logger()
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			l.Error().Err(err).Msg("Failed to read request body")
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer func() {
			if err := r.Body.Close(); err != nil {
				l.Error().Err(err).Msg("Failed to close request body")
			}
		}()

		// Extract headers
		headers := make(map[string]string)
		for name, values := range r.Header {
			if len(values) > 0 {
				headers[name] = values[0]
			}
		}

		// Create webhook request
		webhookReq := &WebhookRequest{
			Headers: headers,
			Payload: body,
		}

		// Call the core webhook processing method
		response, err := s.ReceiveWebhook(webhookReq)
		if err != nil {
			l.Error().Err(err).Msg("Webhook processing failed")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(response); err != nil {
				l.Error().Err(err).Msg("Failed to encode webhook response")
			}
			return
		}

		// Send success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			l.Error().Err(err).Msg("Failed to encode webhook response")
		}
	}
}
