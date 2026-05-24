package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/api"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// version is injected at build time using ldflags
var version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("distributed-rate-limiter %s\n", version)
		os.Exit(0)
	}

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", slog.Any("error", err))
		os.Exit(1)
	}

	// 2. Initialize logger
	logger := config.InitLogger(cfg.Env)
	logger.Info("application starting", slog.String("version", version))

	// 3. Initialize Redis client connection pool (with retries and backoff)
	rdb, err := redis.ConnectWithRetry(cfg)
	if err != nil {
		logger.Error("failed to initialize Redis connection pool", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("failed to close Redis connection pool cleanly", slog.Any("error", err))
		}
	}()

	// Ensure limiter.New() is called to construct the limiter
	limiterCfg := limiter.Config{Algorithm: "fixed_window"}
	if _, err := limiter.New(limiterCfg, rdb); err != nil {
		logger.Error("failed to construct limiter", slog.Any("error", err))
		os.Exit(1)
	}

	// 4. Initialize server
	server := api.NewServer(cfg, version, rdb)

	// 4. Set up channel for graceful shutdown signal coordination
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	serverErrChan := make(chan error, 1)

	// 5. Start the server in a separate goroutine
	go func() {
		if err := server.Start(); err != nil {
			serverErrChan <- err
		}
	}()

	// 6. Block until a signal or a startup error occurs
	select {
	case err := <-serverErrChan:
		logger.Error("server error encountered on startup", slog.Any("error", err))
		os.Exit(1)

	case sig := <-shutdownChan:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))

		// Create a context with shutdown timeout
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", slog.Any("error", err))
			os.Exit(1)
		}
		logger.Info("graceful shutdown completed successfully")
	}
}
