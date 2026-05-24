package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/api"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

var version = "v0.0.0-dev"

func main() {
	// 1. Load Configuration
	appCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger
	config.InitLogger(appCfg.Env, appCfg.LogLevel)
	slog.Info("starting Distributed Rate Limiter", "version", version, "env", appCfg.Env, "port", appCfg.ServerPort)

	// 3. Initialize Redis Client
	rdb, err := redis.ConnectWithRetry(appCfg)
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	// 4. Initialize Rate Limiter Factory
	limCfg := limiter.LimiterConfig{
		Algorithm: appCfg.RateLimitAlgorithm,
		Limit:     appCfg.RateLimitRequests,
		Window:    appCfg.RateLimitWindow,
	}
	
	lim, err := limiter.New(limCfg, rdb)
	if err != nil {
		slog.Error("failed to initialize rate limiter", "error", err)
		os.Exit(1)
	}
	
	// 5. Initialize Metrics & Trackers
	tracker := metrics.NewHotKeyTracker(100) // Threshold of 100 hits for hot keys

	// 6. Initialize and Start HTTP Server
	srv := api.NewServer(appCfg, &limCfg, lim, tracker)

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "port", appCfg.ServerPort)
		serverErrors <- srv.Start()
	}()

	// 7. Graceful Shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case sig := <-shutdown:
		slog.Info("shutdown signal received", "signal", sig)
		
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), appCfg.ShutdownTimeout)
		defer shutdownCancel()
		
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown gracefully", "error", err)
		}
		
		if err := rdb.Close(); err != nil {
			slog.Error("failed to close redis connection", "error", err)
		}
	}

	slog.Info("server exited cleanly")
}
