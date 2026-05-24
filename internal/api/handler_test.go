package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	realredis "github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/redis/go-redis/v9"
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

func TestConfigHandler(t *testing.T) {
	dc := NewDynamicConfig(nil)
	handler := ConfigHandler(dc)

	// Test GET /config
	req := httptest.NewRequest("GET", "/config", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected GET status 200, got %d", w.Code)
	}

	var resp ConfigPayload
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}
	if resp.Limit != 10 || resp.WindowSec != 60 || resp.Algorithm != "fixed_window" {
		t.Errorf("unexpected defaults: %+v", resp)
	}

	// Test PUT /config (valid)
	updatePayload := `{"limit": 20, "window_seconds": 30, "algorithm": "token_bucket"}`
	req = httptest.NewRequest("PUT", "/config", strings.NewReader(updatePayload))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected PUT status 200, got %d", w.Code)
	}

	// Verify GET returns new config
	req = httptest.NewRequest("GET", "/config", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Limit != 20 || resp.WindowSec != 30 || resp.Algorithm != "token_bucket" {
		t.Errorf("config did not update: %+v", resp)
	}

	// Test PUT /config (invalid limit)
	invalidPayload := `{"limit": 0, "window_seconds": 30, "algorithm": "token_bucket"}`
	req = httptest.NewRequest("PUT", "/config", strings.NewReader(invalidPayload))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid limit, got %d", w.Code)
	}

	// Test PUT /config (invalid algorithm)
	invalidPayload = `{"limit": 10, "window_seconds": 30, "algorithm": "unknown"}`
	req = httptest.NewRequest("PUT", "/config", strings.NewReader(invalidPayload))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid algorithm, got %d", w.Code)
	}
}

func TestRateLimitHandler_CheckAndConsume(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdbClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdbClient.Close()

	rdb := realredis.NewClient(rdbClient)
	dc := NewDynamicConfig(rdb)
	limiters := map[string]limiter.Limiter{
		"fixed_window":   limiter.NewFixedWindowLimiter(rdb),
		"sliding_window": limiter.NewSlidingWindowLimiter(rdb),
		"token_bucket":   limiter.NewTokenBucketLimiter(rdb),
		"leaky_bucket":   limiter.NewLeakyBucketLimiter(rdb),
	}

	checkHandler := RateLimitHandler(dc, limiters, true)
	consumeHandler := RateLimitHandler(dc, limiters, false)

	// 1. Test POST /check (dryRun)
	checkPayload := `{"key": "test-key", "algorithm": "fixed_window", "limit": 5, "window_seconds": 10}`
	req := httptest.NewRequest("POST", "/check", strings.NewReader(checkPayload))
	w := httptest.NewRecorder()
	checkHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var checkResp RateCheckResponse
	_ = json.NewDecoder(w.Body).Decode(&checkResp)
	if !checkResp.Allowed {
		t.Error("expected check request to be allowed")
	}
	if checkResp.Remaining != 4 {
		t.Errorf("expected remaining=4, got %d", checkResp.Remaining)
	}

	// Verify no keys created in Redis
	if len(mr.Keys()) > 0 {
		t.Errorf("expected no keys created in redis after check, but found: %v", mr.Keys())
	}

	// 2. Test POST /consume
	req = httptest.NewRequest("POST", "/consume", strings.NewReader(checkPayload))
	w = httptest.NewRecorder()
	consumeHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var consumeResp RateCheckResponse
	_ = json.NewDecoder(w.Body).Decode(&consumeResp)
	if !consumeResp.Allowed {
		t.Error("expected consume request to be allowed")
	}
	if consumeResp.Remaining != 4 {
		t.Errorf("expected remaining=4, got %d", consumeResp.Remaining)
	}

	// Verify key created in Redis
	if len(mr.Keys()) == 0 {
		t.Error("expected keys created in redis after consume")
	}

	// 3. Test fallback to defaults
	fallbackPayload := `{"key": "test-key-fallback"}`
	req = httptest.NewRequest("POST", "/consume", strings.NewReader(fallbackPayload))
	w = httptest.NewRecorder()
	consumeHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	var fallbackResp RateCheckResponse
	_ = json.NewDecoder(w.Body).Decode(&fallbackResp)
	if fallbackResp.Remaining != 9 {
		t.Errorf("expected remaining=9 (from fallback limit 10), got %d", fallbackResp.Remaining)
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

func TestMetricsEndpoint_UpdatesHotKeys(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdbClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdbClient.Close()

	rdb := realredis.NewClient(rdbClient)
	cfg := &config.Config{
		Port:              0,
		Env:               "test",
		ShutdownTimeout:   1 * time.Second,
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       1 * time.Second,
	}

	server := NewServer(cfg, "v0.1.0-test", rdb)

	_, err = rdbClient.ZIncrBy(context.Background(), "rl:hotkeys", 5.0, "key_x").Result()
	if err != nil {
		t.Fatalf("failed to seed ZSET: %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected GET /metrics status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `rate_limiter_hot_key_hits{key="key_x"} 5`) {
		t.Errorf("expected /metrics output to contain 'rate_limiter_hot_key_hits{key=\"key_x\"} 5', got: %s", body)
	}
}
