package api

import (
	"encoding/json"
	"net/http"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
)

// RateCheckRequest represents the JSON payload for /check and /consume endpoints.
type RateCheckRequest struct {
	Key string `json:"key"`
}

// RateCheckResponse represents the JSON response for rate limit checks.
type RateCheckResponse struct {
	Allowed    bool   `json:"allowed"`
	Remaining  int    `json:"remaining"`
	RetryAfter string `json:"retry_after"`
}

// ConfigHandler returns the current static rate limit configuration.
func ConfigHandler(cfg *limiter.LimiterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	}
}

// HealthHandler provides a simple health check endpoint.
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"up"}`))
	}
}

// CheckHandler checks if a key would be allowed WITHOUT consuming a token (Dry Run).
func CheckHandler(lim limiter.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RateCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
			return
		}
		if req.Key == "" {
			http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
			return
		}

		allowed, remaining, retryAfter, err := lim.Simulate(r.Context(), req.Key)
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}

		resp := RateCheckResponse{
			Allowed:    allowed,
			Remaining:  remaining,
			RetryAfter: retryAfter.String(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ConsumeHandler checks AND consumes a token for the given key.
func ConsumeHandler(lim limiter.Limiter, tracker *metrics.HotKeyTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RateCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
			return
		}
		if req.Key == "" {
			http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
			return
		}

		allowed, remaining, retryAfter, err := lim.Check(r.Context(), req.Key)
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}

		// Record the hot key access for observability
		tracker.Record(req.Key)

		resp := RateCheckResponse{
			Allowed:    allowed,
			Remaining:  remaining,
			RetryAfter: retryAfter.String(),
		}

		w.Header().Set("Content-Type", "application/json")
		if !allowed {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		json.NewEncoder(w).Encode(resp)
	}
}
