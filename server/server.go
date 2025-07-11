// Package server hosts the HTTP server for the branch-out application.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/github"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/trunk"
)

// Server is the HTTP server for the branch-out application.
type Server struct {
	Addr string
	Port int

	logger  zerolog.Logger
	server  *http.Server
	config  config.Config
	version string

	jiraClient   jira.IClient
	trunkClient  trunk.IClient
	githubClient github.IClient

	started atomic.Bool
	err     error
}

type options struct {
	config  config.Config
	logger  zerolog.Logger
	version string

	jiraClient   jira.IClient
	trunkClient  trunk.IClient
	githubClient github.IClient
}

// CreateClients creates the clients for reaching out to external services.
func CreateClients(logger zerolog.Logger, config config.Config) (jira.IClient, trunk.IClient, github.IClient, error) {
	jiraClient, err := jira.NewClient(jira.WithLogger(logger), jira.WithConfig(config))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create Jira client: %w", err)
	}
	trunkClient, err := trunk.NewClient(trunk.WithLogger(logger), trunk.WithConfig(config))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create Trunk client: %w", err)
	}
	githubClient, err := github.NewClient(github.WithLogger(logger), github.WithConfig(config))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}
	return jiraClient, trunkClient, githubClient, nil
}

// Option is a functional option that configures the server.
// Default options are used if no options are provided.
type Option func(*options)

// WithJiraClient sets the Jira client for the server.
// This overrides using the config to create a client.
// Useful for testing.
func WithJiraClient(client jira.IClient) Option {
	return func(opts *options) {
		opts.jiraClient = client
	}
}

// WithTrunkClient sets the Trunk client for the server.
// This overrides using the config to create a client.
// Useful for testing.
func WithTrunkClient(client trunk.IClient) Option {
	return func(opts *options) {
		opts.trunkClient = client
	}
}

// WithGitHubClient sets the GitHub client for the server.
// This overrides using the config to create a client.
// Useful for testing.
func WithGitHubClient(client github.IClient) Option {
	return func(opts *options) {
		opts.githubClient = client
	}
}

// WithConfig sets the config for the server.
// Default config is used if no config is provided.
func WithConfig(cfg config.Config) Option {
	return func(opts *options) {
		opts.config = cfg
	}
}

// WithLogger sets the logger for the server.
func WithLogger(logger zerolog.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

// defaultOptions returns the default options for the server.
func defaultOptions() *options {
	return &options{
		logger:  zerolog.Nop(),
		version: "unknown",
	}
}

// New creates a new Server.
func New(options ...Option) (*Server, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	var (
		jiraClient   jira.IClient
		trunkClient  trunk.IClient
		githubClient github.IClient
		err          error
	)

	if opts.jiraClient == nil || opts.trunkClient == nil || opts.githubClient == nil {
		jiraClient, trunkClient, githubClient, err = CreateClients(opts.logger, opts.config)
		if err != nil {
			return nil, fmt.Errorf("failed to create clients: %w", err)
		}
		if opts.jiraClient == nil {
			opts.jiraClient = jiraClient
		}
		if opts.trunkClient == nil {
			opts.trunkClient = trunkClient
		}
		if opts.githubClient == nil {
			opts.githubClient = githubClient
		}
	}

	return &Server{
		Port: opts.config.Port,

		logger:  opts.logger,
		config:  opts.config,
		version: config.Version,

		jiraClient:   opts.jiraClient,
		trunkClient:  opts.trunkClient,
		githubClient: opts.githubClient,
	}, nil
}

// Error returns the error that occurred during server startup.
// It is nil if the server started successfully.
func (s *Server) Error() error {
	return s.err
}

// Start starts the server and blocks until shutdown.
// It handles both programmatic shutdown (via context) and OS signals.
func (s *Server) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Create the appropriate listener
	var (
		listener net.Listener
		err      error
		url      string
	)

	listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		s.err = fmt.Errorf("failed to create listener: %w", err)
		return s.err
	}
	url = listener.Addr().String()
	s.Addr = url

	// Get the port from the listener
	port := listener.Addr().(*net.TCPAddr).Port
	s.Port = port

	baseMux := http.NewServeMux()
	baseMux.HandleFunc("/", indexHandler(s))
	baseMux.HandleFunc("/health", healthHandler(s))

	webhookMux := http.NewServeMux()
	webhookMux.HandleFunc("/", webhookIndexHandler(s))
	webhookMux.HandleFunc("/trunk", webhookHandler(s))
	baseMux.Handle("/webhooks/", webhookMux)

	// Wrap in logging middleware
	handler := s.loggingMiddleware(baseMux)

	s.server = &http.Server{
		Addr:    url,
		Handler: handler,
	}
	s.server.RegisterOnShutdown(func() {
		s.started.Store(false)
	})

	// Listen for OS signals to shutdown the server
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		s.logger.Info().
			Int("port", s.Port).
			Str("addr", s.Addr).
			Msg("Server listening for requests")
		fmt.Println("Listening on", url)
		s.started.Store(true)
		serverErr := s.server.Serve(listener)
		if serverErr != nil && serverErr != http.ErrServerClosed {
			serverErrChan <- serverErr
		}
		s.logger.Info().Msg("Server stopped")
	}()

	// Wait for shutdown signal
	select {
	case err := <-serverErrChan:
		s.logger.Error().Err(err).Msg("Server error")
		s.err = err
		return s.err
	case sig := <-sigChan:
		s.logger.Warn().Str("signal", sig.String()).Msg("Received shutdown signal")
	case <-ctx.Done():
		s.logger.Warn().Msg("Context cancelled, server shutting down")
	}

	err = s.shutdown()
	if err != nil {
		s.err = err
		return s.err
	}
	return nil
}

// WaitHealthy blocks until the server is healthy or the context is done.
func (s *Server) WaitHealthy(ctx context.Context) error {
	health, err := s.Health()
	if err != nil {
		return err
	}
	if health.Status == "healthy" {
		return nil
	}

	timer := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
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
	// Add additional health checks here as relevant.
	if !s.started.Load() {
		s.logger.Warn().Msg("Server unhealthy")
		return HealthResponse{
			Status:    "unhealthy",
			Timestamp: time.Now(),
		}, nil
	}

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
	}

	s.logger.Trace().Str("status", response.Status).Msg("Server healthy")

	return response, nil
}

// WebhookResponse represents the response from webhook processing
type WebhookResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   error  `json:"error,omitempty"`
}

// ReceiveWebhook processes webhook data and returns the result.
func (s *Server) ReceiveWebhook(req *http.Request) (*WebhookResponse, error) {
	l := s.logger.With().
		Str("endpoint", string(req.URL.Path)).
		Int("payload_size_bytes", int(req.ContentLength)).
		Logger()
	l.Debug().Msg("Processing webhook call")

	switch req.URL.Path {
	case "/webhooks/trunk":
		trunkSigningSecret := s.config.Trunk.WebhookSecret
		// Pass nil for jiraClient and trunkClient for now - these can be injected in the future
		return nil, trunk.ReceiveWebhook(l, req, trunkSigningSecret, s.jiraClient, s.trunkClient, s.githubClient)
	default:
		return nil, fmt.Errorf("unknown webhook endpoint: %s", req.URL.Path)
	}
}

// HTTP Handlers - These are thin wrappers around the core methods

func indexHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		info := map[string]any{
			"app":     "branch-out",
			"version": s.version,
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

		// Call the core webhook processing method
		response, err := s.ReceiveWebhook(r)
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

// responseWriter wraps http.ResponseWriter to capture status code and response size for server logging middleware
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

// WriteHeader writes the status code to the response writer
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write writes the data to the response writer
func (rw *responseWriter) Write(data []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(data)
	rw.size += size
	return size, err
}
