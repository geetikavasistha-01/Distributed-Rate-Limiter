package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
)

// DummyLimiter implements the limiter.Limiter interface for testing
type DummyLimiter struct{}

func (d *DummyLimiter) Check(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return true, 10, 0, nil
}

func (d *DummyLimiter) Simulate(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return true, 10, 0, nil
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := &config.Config{
		ServerPort:        8081, // Port for testing
		Env:               "test",
		ShutdownTimeout:   1 * time.Second,
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       1 * time.Second,
	}

	limCfg := &limiter.LimiterConfig{Algorithm: "fixed_window", Limit: 10, Window: time.Minute}
	tracker := metrics.NewHotKeyTracker(100)
	
	server := NewServer(cfg, limCfg, &DummyLimiter{}, tracker)

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Wait briefly to ensure server starts
	time.Sleep(100 * time.Millisecond)

	// Perform a request to health endpoint
	resp, err := http.Get("http://localhost:8081/health")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Trigger graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Verify server exited cleanly
	serverErr := <-errCh
	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		t.Errorf("expected ErrServerClosed, got %v", serverErr)
	}
}
