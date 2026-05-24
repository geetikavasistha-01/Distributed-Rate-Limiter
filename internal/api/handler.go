package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// HealthResponse represents the JSON payload structure of the health endpoint.
type HealthResponse struct {
	Status  string            `json:"status"`
	Service string            `json:"service"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks"`
}

// HealthHandler returns an HTTP handler function for the health endpoint.
func HealthHandler(version string, rdb redis.RedisClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		redisStatus := "healthy"
		httpStatus := http.StatusOK
		status := "ok"

		// Query Redis liveness using the context propagated from middlewares
		if err := rdb.Ping(r.Context()); err != nil {
			redisStatus = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			status = "error"
		}

		w.WriteHeader(httpStatus)

		resp := HealthResponse{
			Status:  status,
			Service: "distributed-rate-limiter",
			Version: version,
			Checks: map[string]string{
				"redis": redisStatus,
			},
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}

// Lua scripts to get and set config atomically in Redis.
const GetConfigLuaScript = `
local key = KEYS[1]
local limit = redis.call("HGET", key, "limit")
local window = redis.call("HGET", key, "window_seconds")
local algo = redis.call("HGET", key, "algorithm")
if not limit or not window or not algo then
    return nil
end
return {limit, window, algo}
`

const SetConfigLuaScript = `
local key = KEYS[1]
redis.call("HSET", key, "limit", ARGV[1], "window_seconds", ARGV[2], "algorithm", ARGV[3])
return 1
`

// DynamicConfig holds dynamic configuration for default rate limit parameters.
type DynamicConfig struct {
	mu        sync.RWMutex
	rdb       redis.RedisClient
	limit     int64
	windowSec int
	algorithm string
}

// NewDynamicConfig creates a new DynamicConfig with default values.
func NewDynamicConfig(rdb redis.RedisClient) *DynamicConfig {
	return &DynamicConfig{
		rdb:       rdb,
		limit:     10,
		windowSec: 60,
		algorithm: "fixed_window",
	}
}

// Get returns the current configuration values, checking Redis first and falling back to local memory.
func (dc *DynamicConfig) Get(ctx context.Context) (int64, int, string) {
	if dc.rdb != nil {
		res, err := dc.rdb.Eval(ctx, GetConfigLuaScript, []string{"rl:config"})
		if err == nil && res != nil {
			if slice, ok := res.([]interface{}); ok && len(slice) >= 3 {
				limitStr, ok1 := slice[0].(string)
				windowStr, ok2 := slice[1].(string)
				algo, ok3 := slice[2].(string)
				if ok1 && ok2 && ok3 {
					limit, err1 := strconv.ParseInt(limitStr, 10, 64)
					window, err2 := strconv.Atoi(windowStr)
					if err1 == nil && err2 == nil {
						dc.mu.Lock()
						dc.limit = limit
						dc.windowSec = window
						dc.algorithm = algo
						dc.mu.Unlock()
						return limit, window, algo
					}
				}
			}
		}
	}

	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.limit, dc.windowSec, dc.algorithm
}

// Set updates the configuration values in Redis and local memory.
func (dc *DynamicConfig) Set(ctx context.Context, limit int64, windowSec int, algorithm string) {
	dc.mu.Lock()
	dc.limit = limit
	dc.windowSec = windowSec
	dc.algorithm = algorithm
	dc.mu.Unlock()

	if dc.rdb != nil {
		_, _ = dc.rdb.Eval(ctx, SetConfigLuaScript, []string{"rl:config"}, limit, windowSec, algorithm)
	}
}

// ConfigPayload is the request/response structure for /config endpoints.
type ConfigPayload struct {
	Limit     int64  `json:"limit"`
	WindowSec int    `json:"window_seconds"`
	Algorithm string `json:"algorithm"`
}

// ConfigHandler returns the HTTP handler for GET and PUT /config.
func ConfigHandler(dc *DynamicConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			limit, windowSec, algo := dc.Get(r.Context())
			resp := ConfigPayload{
				Limit:     limit,
				WindowSec: windowSec,
				Algorithm: algo,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == http.MethodPut {
			var req ConfigPayload
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
				return
			}

			// Validate
			if req.Limit <= 0 {
				http.Error(w, `{"error":"limit must be greater than 0"}`, http.StatusBadRequest)
				return
			}
			if req.WindowSec <= 0 {
				http.Error(w, `{"error":"window_seconds must be greater than 0"}`, http.StatusBadRequest)
				return
			}
			switch req.Algorithm {
			case "fixed_window", "sliding_window", "token_bucket", "leaky_bucket":
				// valid
			default:
				http.Error(w, `{"error":"unsupported algorithm"}`, http.StatusBadRequest)
				return
			}

			dc.Set(r.Context(), req.Limit, req.WindowSec, req.Algorithm)

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(req)
			return
		}

		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// RateCheckRequest is the payload structure for POST /check and POST /consume.
type RateCheckRequest struct {
	Key           string `json:"key"`
	Algorithm     string `json:"algorithm"`
	Limit         int64  `json:"limit"`
	WindowSeconds int    `json:"window_seconds"`
}

// RateCheckResponse is the JSON response structure for rate check results.
type RateCheckResponse struct {
	Allowed   bool      `json:"allowed"`
	Remaining int64     `json:"remaining"`
	ResetTime time.Time `json:"reset_time"`
}

// RateLimitHandler handles POST /check (dry-run) and POST /consume (mutating) checks.
func RateLimitHandler(dc *DynamicConfig, limiters map[string]limiter.Limiter, dryRun bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req RateCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
			return
		}

		if req.Key == "" {
			http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
			return
		}

		// Fallback to default configuration values if they are omitted or 0
		limitVal, windowSecVal, algoVal := dc.Get(r.Context())

		limit := req.Limit
		if limit <= 0 {
			limit = limitVal
		}

		windowSec := req.WindowSeconds
		if windowSec <= 0 {
			windowSec = windowSecVal
		}

		algo := req.Algorithm
		if algo == "" {
			algo = algoVal
		}

		lim, ok := limiters[algo]
		if !ok {
			http.Error(w, `{"error":"unsupported algorithm"}`, http.StatusBadRequest)
			return
		}

		cfg := limiter.LimitConfig{
			Limit:  limit,
			Window: time.Duration(windowSec) * time.Second,
			DryRun: dryRun,
		}

		res, err := lim.Allow(r.Context(), req.Key, cfg)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		resp := RateCheckResponse{
			Allowed:   res.Allowed,
			Remaining: res.Remaining,
			ResetTime: res.ResetTime.UTC(),
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
