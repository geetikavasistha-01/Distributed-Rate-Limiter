package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
)

func TestServerGracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		Port:              0, // Ephemeral port allocated dynamically by the OS
		Env:               "test",
		ShutdownTimeout:   1 * time.Second,
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       1 * time.Second,
	}

	mockRdb := &MockRedisClient{}
	server := NewServer(cfg, "v0.1.0-test", mockRdb)

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Give server a short moment to start and bind
	time.Sleep(100 * time.Millisecond)

	// Invoke Shutdown sequence
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown server gracefully: %v", err)
	}

	// Ensure Start() returned cleanly (since server stopped, it should stop blocking)
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("expected clean server exit, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for server to exit after shutdown call")
	}
}

func TestServer_MetricsEndpoint(t *testing.T) {
	cfg := &config.Config{
		Port:              0,
		Env:               "test",
		ShutdownTimeout:   1 * time.Second,
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       1 * time.Second,
	}

	mockRdb := &MockRedisClient{}
	server := NewServer(cfg, "v0.1.0-test", mockRdb)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected GET /metrics status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "rate_limiter_requests_total") {
		t.Error("expected /metrics output to contain 'rate_limiter_requests_total'")
	}
}
