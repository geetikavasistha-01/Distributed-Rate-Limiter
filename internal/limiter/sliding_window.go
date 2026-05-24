package limiter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// SlidingWindowLuaScript is the Lua script executed atomically in Redis.
// It prunes values older than (now - window), counts the size of the set,
// checks if the request exceeds the limit, adds the current request timestamp,
// and finds the oldest remaining element in the sliding window to compute the exact reset time.
const SlidingWindowLuaScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

local clear_before = now - window

-- 1. Prune expired entries older than (now - window)
redis.call("ZREMRANGEBYSCORE", key, "-inf", clear_before)

-- 2. Count current elements inside the sliding window
local current_requests = redis.call("ZCARD", key)

local allowed = false
if current_requests < limit then
    -- 3. Add current timestamp unique member to the sliding log
    redis.call("ZADD", key, now, member)
    current_requests = current_requests + 1
    allowed = true
end

-- 4. Set key expiration to keep ZSET alive
redis.call("EXPIRE", key, math.ceil(window / 1000))

-- 5. Query oldest timestamp in the sliding window to determine reset boundary
local oldest_with_score = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
local oldest_ts = now
if #oldest_with_score > 0 then
    oldest_ts = tonumber(oldest_with_score[2])
end
local reset_ms = oldest_ts + window

local remaining = limit - current_requests
if remaining < 0 then
    remaining = 0
end

return {allowed and 1 or 0, remaining, reset_ms}
`

// SlidingWindowLimiter implements the Limiter interface using a Sliding Window Log.
type SlidingWindowLimiter struct {
	rdb      redis.RedisClient
	timeFunc func() time.Time // Injectable clock provider for deterministic testing
}

// NewSlidingWindowLimiter instantiates a new SlidingWindowLimiter.
func NewSlidingWindowLimiter(rdb redis.RedisClient) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		rdb:      rdb,
		timeFunc: time.Now,
	}
}

// GenerateUniqueMember builds a unique string member for the Redis ZSET.
// Using unique members is critical: under high concurrency/same-millisecond requests,
// storing only raw timestamps as members would overwrite existing entries,
// bypassing the ZCARD rate limit check.
func GenerateUniqueMember(timestampMs int64) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to simple nanosecond seed if entropy fails
		return fmt.Sprintf("%d-%d", timestampMs, time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", timestampMs, hex.EncodeToString(bytes))
}

// Allow evaluates a rate limit request using a sliding log of timestamps inside a Redis ZSET.
func (s *SlidingWindowLimiter) Allow(ctx context.Context, key string, cfg LimitConfig) (*Result, error) {
	now := s.timeFunc()
	nowMs := now.UnixMilli()
	windowMs := cfg.Window.Milliseconds()

	if windowMs <= 0 {
		return nil, fmt.Errorf("window duration must be at least 1 millisecond")
	}

	redisKey := fmt.Sprintf("rl:sliding:%s", key)
	member := GenerateUniqueMember(nowMs)

	// Execute sliding window Lua script atomically in Redis
	res, err := s.rdb.Eval(ctx, SlidingWindowLuaScript, []string{redisKey}, nowMs, windowMs, cfg.Limit, member)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sliding window Lua script: %w", err)
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
