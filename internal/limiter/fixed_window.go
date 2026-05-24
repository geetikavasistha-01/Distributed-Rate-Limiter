package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// FixedWindowLuaScript is the Lua script executed atomically in Redis.
// It increments the key and sets the TTL only on the first increment (count == 1).
// If dry_run is 1, it simulates the increment and check without writing to Redis.
const FixedWindowLuaScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_secs = tonumber(ARGV[2])
local dry_run = tonumber(ARGV[3] or 0)

local count = redis.call("GET", key)
if not count then
    count = 0
else
    count = tonumber(count)
end

if dry_run == 0 then
    count = redis.call("INCRBY", key, 1)
    if count == 1 then
        redis.call("EXPIRE", key, window_secs)
    end
else
    -- Simulate the increment check without mutating Redis state
    count = count + 1
end

local ttl = redis.call("TTL", key)
return {count, ttl}
`

// FixedWindowLimiter implements the Limiter interface using a Fixed Window Counter algorithm.
type FixedWindowLimiter struct {
	rdb redis.RedisClient
}

// NewFixedWindowLimiter instantiates a new FixedWindowLimiter.
func NewFixedWindowLimiter(rdb redis.RedisClient) *FixedWindowLimiter {
	return &FixedWindowLimiter{rdb: rdb}
}

// Allow checks if a request exceeds the configured limit for a given key in a fixed window.
func (f *FixedWindowLimiter) Allow(ctx context.Context, key string, cfg LimitConfig) (*Result, error) {
	start := time.Now()
	now := time.Now()
	windowSeconds := int64(cfg.Window.Seconds())
	if windowSeconds <= 0 {
		return nil, fmt.Errorf("window duration must be at least 1 second")
	}

	// Calculate the current window timestamp bucket
	windowNum := now.Unix() / windowSeconds
	redisKey := fmt.Sprintf("rl:fixed:%s:%d", key, windowNum)

	dryRunVal := 0
	if cfg.DryRun {
		dryRunVal = 1
	}

	// Execute Lua script atomically on Redis
	redisStart := time.Now()
	res, err := f.rdb.Eval(ctx, FixedWindowLuaScript, []string{redisKey}, cfg.Limit, windowSeconds, dryRunVal)
	redisDuration := time.Since(redisStart).Seconds()
	metrics.RedisDuration.WithLabelValues("allow").Observe(redisDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to execute fixed window Lua script: %w", err)
	}

	// Parse Redis response: [count (int64), ttl (int64)]
	slice, ok := res.([]interface{})
	if !ok || len(slice) < 2 {
		return nil, fmt.Errorf("invalid Lua script response type, expected slice of size 2, got %T", res)
	}

	count, ok1 := slice[0].(int64)
	ttl, ok2 := slice[1].(int64)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("failed to parse Lua script elements: count ok=%t, ttl ok=%t", ok1, ok2)
	}

	remaining := cfg.Limit - count
	if remaining < 0 {
		remaining = 0
	}

	// Determine reset time based on key's remaining TTL returned from Redis
	var resetTime time.Time
	if ttl > 0 {
		resetTime = now.Add(time.Duration(ttl) * time.Second)
	} else {
		// Fallback boundary calculation if TTL is unavailable
		resetTime = time.Unix((windowNum+1)*windowSeconds, 0)
	}

	allowed := count <= cfg.Limit
	duration := time.Since(start).Seconds()

	status := "allowed"
	if !allowed {
		status = "blocked"
	}

	// Update Prometheus metrics
	keyType := metrics.GetKeyType(key)
	metrics.RequestsTotal.WithLabelValues("fixed_window", status, keyType).Inc()
	metrics.EvaluationDuration.WithLabelValues("fixed_window", status).Observe(duration)

	// Update hot keys if not dry-run
	if !cfg.DryRun {
		metrics.IncrementHotKey(ctx, f.rdb, key)
	}

	return &Result{
		Allowed:   allowed,
		Remaining: remaining,
		ResetTime: resetTime,
	}, nil
}
