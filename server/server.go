// Package server hosts the HTTP server for the branch-out application.
package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rs/zerolog"
)

// Server is the HTTP server for the branch-out application.
type Server struct {
	logger     zerolog.Logger
	webhookURL string
	port       int
	server     *http.Server
	stop       chan struct{}
}

// New creates a new Server.
func New(logger zerolog.Logger, webhookURL string, port int) *Server {
	return &Server{
		logger:     logger,
		webhookURL: webhookURL,
		port:       port,
		stop:       make(chan struct{}),
	}
}

// Start starts the server.
func (s *Server) Start() error {
	s.logger.Info().Str("webhookURL", s.webhookURL).Int("port", s.port).Msg("Starting server")

	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler(s))
	mux.HandleFunc("/webhook", webhookHandler(s))
	mux.HandleFunc("/health", healthHandler(s))

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		<-s.stop
		if err := s.Stop(); err != nil {
			s.logger.Error().Err(err).Msg("Failed to stop server")
		}
	}()

	if err := s.server.ListenAndServe(); err != nil {
		s.logger.Error().Err(err).Msg("Failed to start server")
		return err
	}

	return nil
}

// Stop stops the server.
func (s *Server) Stop() error {
	close(s.stop)
	return s.server.Shutdown(context.Background())
}

func healthHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		s.logger.Info().Msg("Health check")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to write response")
		}
	}
}

func webhookHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		s.logger.Info().Msg("Received webhook")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("Webhook received"))
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to write response")
		}
	}
}

func indexHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("Hello, World!"))
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to write response")
		}
	}
}
