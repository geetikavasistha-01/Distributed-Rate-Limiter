package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// FixedWindowLuaScript increments the key and sets the TTL only on the first increment.
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

type FixedWindowLimiter struct {
	rdb redis.RedisClient
	cfg LimiterConfig
}

func NewFixedWindowLimiter(rdb redis.RedisClient, cfg LimiterConfig) *FixedWindowLimiter {
	return &FixedWindowLimiter{rdb: rdb, cfg: cfg}
}

func (f *FixedWindowLimiter) Check(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return f.execute(ctx, key, 0)
}

func (f *FixedWindowLimiter) Simulate(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return f.execute(ctx, key, 1)
}

func (f *FixedWindowLimiter) execute(ctx context.Context, key string, dryRun int) (bool, int, time.Duration, error) {
	windowSeconds := int64(f.cfg.Window.Seconds())
	if windowSeconds <= 0 {
		return false, 0, 0, fmt.Errorf("window duration must be at least 1 second")
	}

	redisKey := fmt.Sprintf("rl:fixed_window:%s", key)

	res, err := f.rdb.Eval(ctx, FixedWindowLuaScript, []string{redisKey}, f.cfg.Limit, windowSeconds, dryRun)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to execute fixed window Lua script: %w", err)
	}

	slice, ok := res.([]interface{})
	if !ok || len(slice) < 2 {
		return false, 0, 0, fmt.Errorf("invalid Lua script response type, expected slice of size 2, got %T", res)
	}

	count, ok1 := slice[0].(int64)
	ttlSecs, ok2 := slice[1].(int64)
	if !ok1 || !ok2 {
		return false, 0, 0, fmt.Errorf("failed to parse Lua script elements")
	}

	remaining := int(f.cfg.Limit - count)
	if remaining < 0 {
		remaining = 0
	}

	var retryAfter time.Duration
	if ttlSecs > 0 {
		retryAfter = time.Duration(ttlSecs) * time.Second
	} else if count > f.cfg.Limit {
		retryAfter = f.cfg.Window
	}

	allowed := count <= f.cfg.Limit
	return allowed, remaining, retryAfter, nil
}
