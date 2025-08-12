package processing

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

	"github.com/smartcontractkit/branch-out/aws"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/github"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/telemetry"
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

	jiraClient   JiraClient
	trunkClient  TrunkClient
	githubClient GithubClient
	awsClient    AWSClient

	// Background worker for processing SQS messages
	worker *Worker

	// Telemetry
	metrics *telemetry.Metrics

	running atomic.Bool
	err     error
}

type options struct {
	config  config.Config
	logger  zerolog.Logger
	version string

	jiraClient   JiraClient
	trunkClient  TrunkClient
	githubClient GithubClient
	awsClient    AWSClient
	metrics      *telemetry.Metrics
}

// CreateClients creates the clients for reaching out to external services.
func CreateClients(
	logger zerolog.Logger,
	config config.Config,
	metrics *telemetry.Metrics,
) (JiraClient, TrunkClient, GithubClient, AWSClient, error) {
	jiraClient, err := jira.NewClient(jira.WithLogger(logger), jira.WithConfig(config), jira.WithMetrics(metrics))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create Jira client: %w", err)
	}
	trunkClient, err := trunk.NewClient(trunk.WithLogger(logger), trunk.WithConfig(config), trunk.WithMetrics(metrics))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create Trunk client: %w", err)
	}
	githubClient, err := github.NewClient(
		github.WithLogger(logger),
		github.WithConfig(config),
		github.WithMetrics(metrics),
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}
	awsClient, err := aws.NewClient(aws.WithLogger(logger), aws.WithConfig(config), aws.WithMetrics(metrics))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create AWS client: %w", err)
	}
	return jiraClient, trunkClient, githubClient, awsClient, nil
}

// Option is a functional option that configures the server.
// Default options are used if no options are provided.
type Option func(*options)

// WithJiraClient sets the Jira client for the server.
// This overrides using the config to create a client.
// Useful for testing.
func WithJiraClient(client JiraClient) Option {
	return func(opts *options) {
		opts.jiraClient = client
	}
}

// WithTrunkClient sets the Trunk client for the server.
// This overrides using the config to create a client.
// Useful for testing.
func WithTrunkClient(client TrunkClient) Option {
	return func(opts *options) {
		opts.trunkClient = client
	}
}

// WithGitHubClient sets the GitHub client for the server.
// This overrides using the config to create a client.
// Useful for testing.
func WithGitHubClient(client GithubClient) Option {
	return func(opts *options) {
		opts.githubClient = client
	}
}

// WithAWSClient sets the AWS client for the server.
func WithAWSClient(client AWSClient) Option {
	return func(opts *options) {
		opts.awsClient = client
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

// WithMetrics sets the metrics instance for the server.
func WithMetrics(metrics *telemetry.Metrics) Option {
	return func(opts *options) {
		opts.metrics = metrics
	}
}

// defaultOptions returns the default options for the server.
func defaultOptions() *options {
	return &options{
		logger:  zerolog.Nop(),
		version: "unknown",
	}
}

// NewServer creates a new Server to run the branch-out application.
func NewServer(options ...Option) (*Server, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	var (
		jiraClient   JiraClient
		trunkClient  TrunkClient
		githubClient GithubClient
		awsClient    AWSClient
		err          error
	)

	if opts.jiraClient == nil || opts.trunkClient == nil || opts.githubClient == nil {
		jiraClient, trunkClient, githubClient, awsClient, err = CreateClients(opts.logger, opts.config, opts.metrics)
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
		if opts.awsClient == nil {
			opts.awsClient = awsClient
		}
	}

	// Create the background worker for SQS processing
	workerConfig := Config{
		PollInterval: 15 * time.Second,
	}

	sqsWorker := NewWorker(
		opts.logger,
		opts.awsClient,
		opts.jiraClient,
		opts.trunkClient,
		opts.githubClient,
		opts.metrics,
		workerConfig,
	)

	return &Server{
		Port: opts.config.Port,

		logger:  opts.logger,
		config:  opts.config,
		version: config.Version,

		jiraClient:   opts.jiraClient,
		trunkClient:  opts.trunkClient,
		githubClient: opts.githubClient,
		awsClient:    opts.awsClient,
		worker:       sqsWorker,
		metrics:      opts.metrics,
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
	baseMux.HandleFunc("/", strictIndexHandler(s))
	baseMux.HandleFunc("/health", healthHandler(s))
	baseMux.HandleFunc("/webhooks/", webhookHandler(s))

	// Wrap in logging middleware
	handler := s.loggingMiddleware(baseMux)

	s.server = &http.Server{
		Addr:    url,
		Handler: handler,
	}
	s.server.RegisterOnShutdown(func() {
		s.running.Store(false)
	})

	// Start the background worker for SQS processing
	if s.worker != nil {
		s.logger.Info().Msg("Starting SQS worker")
		if err := s.worker.Start(); err != nil {
			s.logger.Error().Err(err).Msg("Failed to start SQS worker")
			s.err = fmt.Errorf("failed to start SQS worker: %w", err)
			return s.err
		}
	}

	// Listen for OS signals to shutdown the server
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		s.logger.Info().
			Int("port", s.Port).
			Str("addr", s.Addr).
			Msg("Server listening")
		s.running.Store(true)
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
	// Stop the background worker first
	if s.worker != nil {
		s.logger.Info().Msg("Stopping SQS worker")
		if err := s.worker.Stop(); err != nil {
			s.logger.Error().Err(err).Msg("Failed to stop SQS worker")
			// Continue with server shutdown even if worker stop fails
		}
	}

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
	if !s.running.Load() {
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
	Message string `json:"message,omitempty"`
}

// ReceiveWebhook processes webhook data and returns the result.
func (s *Server) ReceiveWebhook(req *http.Request) *WebhookResponse {
	l := s.logger.With().
		Str("endpoint", string(req.URL.Path)).
		Int("payload_size_bytes", int(req.ContentLength)).
		Logger()
	l.Debug().Msg("Processing webhook call")

	var (
		response *WebhookResponse
		err      error
	)
	switch req.URL.Path {
	case "/webhooks/trunk":
		// Create webhook handler for this request
		err = VerifyAndEnqueueWebhook(l, s.config.Trunk.WebhookSecret, s.awsClient, s.metrics, req)
	default:
		err = fmt.Errorf("unknown webhook endpoint: %s", req.URL.Path)
	}

	if err != nil {
		l.Error().Err(err).Msg("Webhook processing failed")
		response = &WebhookResponse{
			Success: false,
			Message: err.Error(),
		}
	} else {
		response = &WebhookResponse{
			Success: true,
			Message: "Webhook processed successfully",
		}
	}

	return response
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

func strictIndexHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only respond to exact "/" requests
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// Call the regular index handler for exact "/" requests
		indexHandler(s)(w, r)
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

func webhookHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := s.logger.With().Str("handler", "webhook").Logger()
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Call the core webhook processing method
		response := s.ReceiveWebhook(r)

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Set status code based on success
		if response.Success {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}

		// Encode and send response
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

		// Record HTTP metrics if available
		s.metrics.IncHTTPRequest(r.Context(), rw.statusCode, r.Method, r.URL.Path)
		if rw.statusCode >= 400 {
			s.metrics.IncHTTPError(r.Context(), rw.statusCode)
		}

		s.logger.Trace().
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Int("status_code", rw.statusCode).
			Int("response_size", rw.size).
			Str("duration", duration.String()).
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
