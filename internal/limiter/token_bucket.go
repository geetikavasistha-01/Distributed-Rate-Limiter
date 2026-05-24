package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// TokenBucketLuaScript implements token bucket in a Redis Hash.
const TokenBucketLuaScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2]) -- tokens per millisecond
local now = tonumber(ARGV[3]) -- current time in milliseconds
local requested = tonumber(ARGV[4]) -- tokens to consume
local ttl = tonumber(ARGV[5]) -- key expiry TTL in seconds
local dry_run = tonumber(ARGV[6] or 0)

local data = redis.call("HMGET", key, "tokens", "last_refilled_at")
local tokens = tonumber(data[1])
local last_refilled_at = tonumber(data[2])

if not tokens then
    tokens = capacity
    last_refilled_at = now
else
    local elapsed = now - last_refilled_at
    if elapsed > 0 then
        local refill = elapsed * refill_rate
        tokens = math.min(capacity, tokens + refill)
    end
end

local allowed = false
local simulated_tokens = tokens
if simulated_tokens >= requested then
    simulated_tokens = simulated_tokens - requested
    allowed = true
end

if dry_run == 0 then
    redis.call("HMSET", key, "tokens", simulated_tokens, "last_refilled_at", now)
    redis.call("EXPIRE", key, ttl)
    tokens = simulated_tokens
else
    tokens = simulated_tokens
end

local remaining = math.floor(tokens)

local missing_tokens = capacity - tokens
local reset_delay_ms = 0
if refill_rate > 0 and missing_tokens > 0 then
    reset_delay_ms = math.ceil(missing_tokens / refill_rate)
end

return {allowed and 1 or 0, remaining, reset_delay_ms}
`

type TokenBucketLimiter struct {
	rdb      redis.RedisClient
	cfg      LimiterConfig
	timeFunc func() time.Time
}

func NewTokenBucketLimiter(rdb redis.RedisClient, cfg LimiterConfig) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rdb:      rdb,
		cfg:      cfg,
		timeFunc: time.Now,
	}
}

func (t *TokenBucketLimiter) Check(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return t.execute(ctx, key, 0)
}

func (t *TokenBucketLimiter) Simulate(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return t.execute(ctx, key, 1)
}

func (t *TokenBucketLimiter) execute(ctx context.Context, key string, dryRun int) (bool, int, time.Duration, error) {
	now := t.timeFunc()
	nowMs := now.UnixMilli()
	windowMs := t.cfg.Window.Milliseconds()

	if windowMs <= 0 {
		return false, 0, 0, fmt.Errorf("window duration must be at least 1 millisecond")
	}

	refillRate := float64(t.cfg.Limit) / float64(windowMs)
	ttlSecs := int64(t.cfg.Window.Seconds())
	if ttlSecs <= 0 {
		ttlSecs = 1
	}

	redisKey := fmt.Sprintf("rl:token_bucket:%s", key)
	requested := 1

	res, err := t.rdb.Eval(ctx, TokenBucketLuaScript, []string{redisKey}, t.cfg.Limit, refillRate, nowMs, requested, ttlSecs, dryRun)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to execute token bucket Lua script: %w", err)
	}

	slice, ok := res.([]interface{})
	if !ok || len(slice) < 3 {
		return false, 0, 0, fmt.Errorf("invalid Lua script response type")
	}

	allowedVal, ok1 := slice[0].(int64)
	remaining64, ok2 := slice[1].(int64)
	resetDelayMs, ok3 := slice[2].(int64)
	if !ok1 || !ok2 || !ok3 {
		return false, 0, 0, fmt.Errorf("failed to parse Lua script elements")
	}

	allowed := allowedVal == 1
	remaining := int(remaining64)
	retryAfter := time.Duration(resetDelayMs) * time.Millisecond

	return allowed, remaining, retryAfter, nil
}
