package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// LeakyBucketLuaScript implements the GCRA algorithm.
const LeakyBucketLuaScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local dry_run = tonumber(ARGV[5] or 0)

local emission_interval = window_ms / capacity
local delay_tolerance = window_ms

local tat = tonumber(redis.call("GET", key))

if not tat then
    tat = now
end

local new_tat = math.max(now, tat) + emission_interval
local delay = new_tat - now

local allowed = false
if delay <= delay_tolerance then
    allowed = true
    if dry_run == 0 then
        redis.call("SET", key, new_tat, "EX", ttl)
    end
else
    new_tat = tat
end

local remaining = math.floor((delay_tolerance - (new_tat - now)) / emission_interval)
if remaining < 0 then
    remaining = 0
end

local reset_delay_ms = new_tat - now
if reset_delay_ms < 0 then
    reset_delay_ms = 0
end

return {allowed and 1 or 0, remaining, reset_delay_ms}
`

type LeakyBucketLimiter struct {
	rdb      redis.RedisClient
	cfg      LimiterConfig
	timeFunc func() time.Time
}

func NewLeakyBucketLimiter(rdb redis.RedisClient, cfg LimiterConfig) *LeakyBucketLimiter {
	return &LeakyBucketLimiter{
		rdb:      rdb,
		cfg:      cfg,
		timeFunc: time.Now,
	}
}

func (l *LeakyBucketLimiter) Check(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return l.execute(ctx, key, 0)
}

func (l *LeakyBucketLimiter) Simulate(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return l.execute(ctx, key, 1)
}

func (l *LeakyBucketLimiter) execute(ctx context.Context, key string, dryRun int) (bool, int, time.Duration, error) {
	now := l.timeFunc()
	nowMs := now.UnixMilli()
	windowMs := l.cfg.Window.Milliseconds()

	if windowMs <= 0 {
		return false, 0, 0, fmt.Errorf("window duration must be at least 1 millisecond")
	}

	redisKey := fmt.Sprintf("rl:leaky_bucket:%s", key)
	ttlSecs := int64(l.cfg.Window.Seconds())
	if ttlSecs <= 0 {
		ttlSecs = 1
	}

	res, err := l.rdb.Eval(ctx, LeakyBucketLuaScript, []string{redisKey}, l.cfg.Limit, windowMs, nowMs, ttlSecs, dryRun)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to execute leaky bucket Lua script: %w", err)
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
