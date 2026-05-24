package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_Healthy(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	version := "v0.1.0"
	mockRdb := &MockRedisClient{} // Defaults to no-op (nil return error = healthy)

	handler := HealthHandler(version, mockRdb)
	handler.ServeHTTP(w, req)

	// Validate status code
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", w.Code)
	}

	// Validate content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	// Validate JSON body
	var resp HealthResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %s", resp.Status)
	}
	if resp.Service != "distributed-rate-limiter" {
		t.Errorf("expected service=distributed-rate-limiter, got %s", resp.Service)
	}
	if resp.Version != version {
		t.Errorf("expected version=%s, got %s", version, resp.Version)
	}
	if resp.Checks["redis"] != "healthy" {
		t.Errorf("expected checks.redis=healthy, got %s", resp.Checks["redis"])
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	version := "v0.1.0"
	mockRdb := &MockRedisClient{
		PingFunc: func(ctx context.Context) error {
			return errors.New("redis connection refused")
		},
	}

	handler := HealthHandler(version, mockRdb)
	handler.ServeHTTP(w, req)

	// Validate status code
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 Service Unavailable, got %d", w.Code)
	}

	// Validate content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	// Validate JSON body
	var resp HealthResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("expected status=error, got %s", resp.Status)
	}
	if resp.Checks["redis"] != "unhealthy" {
		t.Errorf("expected checks.redis=unhealthy, got %s", resp.Checks["redis"])
	}
}
