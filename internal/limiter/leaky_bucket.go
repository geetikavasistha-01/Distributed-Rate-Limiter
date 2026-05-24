package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// LeakyBucketLuaScript is the Lua script executed atomically in Redis.
// It implements the Generic Cell Rate Algorithm (GCRA) for traffic shaping.
// It calculates Theoretical Arrival Time (tat) offsets to verify if requests
// fall within the virtual delay tolerance limits, and sets key expiration accordingly.
const LeakyBucketLuaScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local dry_run = tonumber(ARGV[5] or 0)

local emission_interval = window_ms / capacity
local delay_tolerance = window_ms

-- 1. Fetch current Theoretical Arrival Time (tat)
local tat = tonumber(redis.call("GET", key))

if not tat then
    tat = now
end

-- 2. Calculate virtual queue arrival time
local new_tat = math.max(now, tat) + emission_interval
local delay = new_tat - now

local allowed = false
if delay <= delay_tolerance then
    allowed = true
    -- 3. Update tat in Redis with expiry if dry_run is disabled
    if dry_run == 0 then
        redis.call("SET", key, new_tat, "EX", ttl)
    end
else
    -- Request blocked, keep existing tat boundary
    new_tat = tat
end

-- 4. Calculate remaining quota slots
local remaining = math.floor((delay_tolerance - (new_tat - now)) / emission_interval)
if remaining < 0 then
    remaining = 0
end

-- 5. Reset time is when the virtual queue becomes completely empty (tat = now)
local reset_ms = new_tat

return {allowed and 1 or 0, remaining, reset_ms}
`

// LeakyBucketLimiter implements the Limiter interface using a GCRA Leaky Bucket algorithm.
type LeakyBucketLimiter struct {
	rdb      redis.RedisClient
	timeFunc func() time.Time // Injectable clock provider for deterministic testing
}

// NewLeakyBucketLimiter instantiates a new LeakyBucketLimiter.
func NewLeakyBucketLimiter(rdb redis.RedisClient) *LeakyBucketLimiter {
	return &LeakyBucketLimiter{
		rdb:      rdb,
		timeFunc: time.Now,
	}
}

// Allow evaluates a rate limit check by scheduling request emissions under GCRA.
func (l *LeakyBucketLimiter) Allow(ctx context.Context, key string, cfg LimitConfig) (*Result, error) {
	start := time.Now()
	now := l.timeFunc()
	nowMs := now.UnixMilli()
	windowMs := cfg.Window.Milliseconds()

	if windowMs <= 0 {
		return nil, fmt.Errorf("window duration must be at least 1 millisecond")
	}

	redisKey := fmt.Sprintf("rl:leaky:%s", key)
	ttlSecs := int64(cfg.Window.Seconds())
	if ttlSecs <= 0 {
		ttlSecs = 1
	}

	dryRunVal := 0
	if cfg.DryRun {
		dryRunVal = 1
	}

	// Execute GCRA Lua script atomically in Redis
	redisStart := time.Now()
	res, err := l.rdb.Eval(ctx, LeakyBucketLuaScript, []string{redisKey}, cfg.Limit, windowMs, nowMs, ttlSecs, dryRunVal)
	redisDuration := time.Since(redisStart).Seconds()
	metrics.RedisDuration.WithLabelValues("allow").Observe(redisDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to execute leaky bucket Lua script: %w", err)
	}

	// Parse Redis response: [allowed (int64), remaining (int64), resetTimeMs (int64)]
	slice, ok := res.([]interface{})
	if !ok || len(slice) < 3 {
		return nil, fmt.Errorf("invalid Lua script response type, expected slice of size 3, got %T", res)
	}

	allowedVal, ok1 := slice[0].(int64)
	remaining, ok2 := slice[1].(int64)
	resetMs, ok3 := slice[2].(int64)
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("failed to parse Lua script elements: allowed ok=%t, remaining ok=%t, resetMs ok=%t", ok1, ok2, ok3)
	}

	allowed := allowedVal == 1
	duration := time.Since(start).Seconds()

	status := "allowed"
	if !allowed {
		status = "blocked"
	}

	// Update Prometheus metrics
	keyType := metrics.GetKeyType(key)
	metrics.RequestsTotal.WithLabelValues("leaky_bucket", status, keyType).Inc()
	metrics.EvaluationDuration.WithLabelValues("leaky_bucket", status).Observe(duration)

	// Update hot keys if not dry-run
	if !cfg.DryRun {
		metrics.IncrementHotKey(ctx, l.rdb, key)
	}

	return &Result{
		Allowed:   allowed,
		Remaining: remaining,
		ResetTime: time.UnixMilli(resetMs),
	}, nil
}
