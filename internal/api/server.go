package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/middleware"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// Server encapsulates the HTTP server logic.
type Server struct {
	httpServer *http.Server
	cfg        *config.Config
}

// NewServer configures and returns a Server instance with configured timeouts, routing, and middlewares.
func NewServer(cfg *config.Config, version string, rdb redis.RedisClient) *Server {
	mux := http.NewServeMux()

	// Register health check endpoint (using Go 1.22+ routing enhancements)
	mux.HandleFunc("GET /health", HealthHandler(version, rdb))

	// Chain middlewares: RequestID (outer) -> Recovery (inner) -> ServeMux (target)
	var handler http.Handler = mux
	handler = middleware.Recovery(handler)
	handler = middleware.RequestID(handler)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	return &Server{
		httpServer: httpServer,
		cfg:        cfg,
	}
}

// Start runs the HTTP server. This call blocks until the server stops or encounters an error.
func (s *Server) Start() error {
	slog.Info("starting HTTP server", slog.String("addr", s.httpServer.Addr), slog.String("env", s.cfg.Env))
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down HTTP server gracefully")
	return s.httpServer.Shutdown(ctx)
}
