package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/middleware"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server encapsulates the HTTP server logic.
type Server struct {
	httpServer *http.Server
	cfg        *config.Config
}

// NewServer configures and returns a Server instance with configured timeouts, routing, and middlewares.
func NewServer(cfg *config.Config, version string, rdb redis.RedisClient) *Server {
	mux := http.NewServeMux()

	// Initialize dynamic configuration manager
	dc := NewDynamicConfig(rdb)

	// Initialize metrics collectors
	metrics.Init()

	// Initialize the map of available limiters
	limiters := map[string]limiter.Limiter{
		"fixed_window":   limiter.NewFixedWindowLimiter(rdb),
		"sliding_window": limiter.NewSlidingWindowLimiter(rdb),
		"token_bucket":   limiter.NewTokenBucketLimiter(rdb),
		"leaky_bucket":   limiter.NewLeakyBucketLimiter(rdb),
	}

	// Register health check endpoint (using Go 1.22+ routing enhancements)
	mux.HandleFunc("GET /health", HealthHandler(version, rdb))

	// Register dynamic config endpoints
	mux.HandleFunc("GET /config", ConfigHandler(dc))
	mux.HandleFunc("PUT /config", ConfigHandler(dc))

	// Register rate limiting evaluation endpoints
	mux.HandleFunc("POST /check", RateLimitHandler(dc, limiters, true))
	mux.HandleFunc("POST /consume", RateLimitHandler(dc, limiters, false))

	// Register metrics endpoint
	mux.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.UpdateHotKeys(r.Context(), rdb)
		promhttp.Handler().ServeHTTP(w, r)
	}))

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
