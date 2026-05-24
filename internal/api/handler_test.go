package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
	realredis "github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/redis/go-redis/v9"
)

func TestHealthHandler_Healthy(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler := HealthHandler()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", w.Code)
	}
}

func TestConfigHandler(t *testing.T) {
	cfg := &limiter.LimiterConfig{
		Algorithm: "fixed_window",
		Limit:     10,
		Window:    time.Minute,
	}
	handler := ConfigHandler(cfg)

	req := httptest.NewRequest("GET", "/config", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected GET status 200, got %d", w.Code)
	}

	var resp limiter.LimiterConfig
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}
	if resp.Limit != 10 || resp.Algorithm != "fixed_window" {
		t.Errorf("unexpected config defaults: %+v", resp)
	}
}

func TestCheckAndConsumeHandlers(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdbClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdbClient.Close()
	rdb := realredis.NewClient(rdbClient)

	cfg := limiter.LimiterConfig{Algorithm: "fixed_window", Limit: 5, Window: 10 * time.Second}
	lim := limiter.NewFixedWindowLimiter(rdb, cfg)
	tracker := metrics.NewHotKeyTracker(5)

	checkHandler := CheckHandler(lim)
	consumeHandler := ConsumeHandler(lim, tracker)

	payload := `{"key": "test-key"}`

	// 1. Test POST /check (dryRun)
	req := httptest.NewRequest("POST", "/check", strings.NewReader(payload))
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

	// 2. Test POST /consume
	req = httptest.NewRequest("POST", "/consume", strings.NewReader(payload))
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
	if !tracker.IsHot("test-key") && tracker.TopKeys(1)[0] == "test-key" {
		t.Log("Tracker correctly recorded key access")
	}
}
