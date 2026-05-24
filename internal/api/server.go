package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server represents the HTTP API server
type Server struct {
	httpServer *http.Server
}

// NewServer initializes and wires up the HTTP server, middleware, and handlers.
func NewServer(appCfg *config.Config, limCfg *limiter.LimiterConfig, lim limiter.Limiter, tracker *metrics.HotKeyTracker) *Server {
	mux := http.NewServeMux()

	// Register Handlers
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /health", HealthHandler())
	mux.HandleFunc("GET /config", ConfigHandler(limCfg))
	mux.HandleFunc("POST /check", CheckHandler(lim))
	mux.HandleFunc("POST /consume", ConsumeHandler(lim, tracker))

	// Wire Middleware
	// Order: Recovery (outer) -> RequestID -> RateLimit (inner)
	handler := middleware.RateLimit(lim, limCfg)(mux)
	handler = middleware.RequestID(handler)
	handler = middleware.Recovery(handler)

	addr := fmt.Sprintf(":%d", appCfg.ServerPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       appCfg.ReadTimeout,
		ReadHeaderTimeout: appCfg.ReadHeaderTimeout,
		WriteTimeout:      appCfg.WriteTimeout,
		IdleTimeout:       appCfg.IdleTimeout,
	}

	return &Server{
		httpServer: srv,
	}
}

// Start runs the HTTP server.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
