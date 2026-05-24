package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// TokenBucketLuaScript is the Lua script executed atomically in Redis.
// It stores the bucket state (tokens, last_refilled_at) in a Redis Hash.
// On every check, it calculates refilled tokens based on the elapsed time,
// decrements if allowed, and calculates the exact timestamp when the bucket refills fully.
const TokenBucketLuaScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2]) -- tokens per millisecond
local now = tonumber(ARGV[3]) -- current time in milliseconds
local requested = tonumber(ARGV[4]) -- tokens to consume
local ttl = tonumber(ARGV[5]) -- key expiry TTL in seconds

-- Get current bucket state from Hash
local data = redis.call("HMGET", key, "tokens", "last_refilled_at")
local tokens = tonumber(data[1])
local last_refilled_at = tonumber(data[2])

if not tokens then
    -- Initialize bucket to full capacity
    tokens = capacity
    last_refilled_at = now
else
    -- Calculate tokens refilled based on elapsed milliseconds
    local elapsed = now - last_refilled_at
    if elapsed > 0 then
        local refill = elapsed * refill_rate
        tokens = math.min(capacity, tokens + refill)
        last_refilled_at = now
    end
end

local allowed = false
if tokens >= requested then
    tokens = tokens - requested
    allowed = true
end

-- Save updated bucket state
redis.call("HMSET", key, "tokens", tokens, "last_refilled_at", last_refilled_at)
redis.call("EXPIRE", key, ttl)

local remaining = math.floor(tokens)

-- Reset time: timestamp (ms) when the bucket will be completely full
local missing_tokens = capacity - tokens
local reset_ms = now
if refill_rate > 0 and missing_tokens > 0 then
    reset_ms = now + math.ceil(missing_tokens / refill_rate)
end

return {allowed and 1 or 0, remaining, reset_ms}
`

// TokenBucketLimiter implements the Limiter interface using the Token Bucket algorithm.
type TokenBucketLimiter struct {
	rdb      redis.RedisClient
	timeFunc func() time.Time // Injectable clock provider for deterministic testing
}

// NewTokenBucketLimiter instantiates a new TokenBucketLimiter.
func NewTokenBucketLimiter(rdb redis.RedisClient) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rdb:      rdb,
		timeFunc: time.Now,
	}
}

// Allow checks if a request is permitted by consuming a token from the bucket.
// Tokens refill continuously over time based on the limit/window ratio.
func (t *TokenBucketLimiter) Allow(ctx context.Context, key string, cfg LimitConfig) (*Result, error) {
	now := t.timeFunc()
	nowMs := now.UnixMilli()
	windowMs := cfg.Window.Milliseconds()

	if windowMs <= 0 {
		return nil, fmt.Errorf("window duration must be at least 1 millisecond")
	}

	// Refill rate: tokens refilled per millisecond
	refillRate := float64(cfg.Limit) / float64(windowMs)
	ttlSecs := int64(cfg.Window.Seconds())
	if ttlSecs <= 0 {
		ttlSecs = 1
	}

	redisKey := fmt.Sprintf("rl:token_bucket:%s", key)
	requested := 1

	// Execute Token Bucket Lua script atomically in Redis
	res, err := t.rdb.Eval(ctx, TokenBucketLuaScript, []string{redisKey}, cfg.Limit, refillRate, nowMs, requested, ttlSecs)
	if err != nil {
		return nil, fmt.Errorf("failed to execute token bucket Lua script: %w", err)
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

	return &Result{
		Allowed:   allowedVal == 1,
		Remaining: remaining,
		ResetTime: time.UnixMilli(resetMs),
	}, nil
}
