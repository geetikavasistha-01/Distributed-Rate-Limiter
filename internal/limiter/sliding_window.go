package limiter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// SlidingWindowLuaScript evaluates rate limits using a sliding window log in a Redis ZSET.
const SlidingWindowLuaScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local dry_run = tonumber(ARGV[5] or 0)

local clear_before = now - window

local allowed = false
local current_requests = 0
local oldest_ts = now

if dry_run == 0 then
    -- Prune expired entries
    redis.call("ZREMRANGEBYSCORE", key, "-inf", clear_before)
    current_requests = redis.call("ZCARD", key)

    if current_requests < limit then
        redis.call("ZADD", key, now, member)
        current_requests = current_requests + 1
        allowed = true
    end

    redis.call("EXPIRE", key, math.ceil(window / 1000))

    local oldest_with_score = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
    if #oldest_with_score > 0 then
        oldest_ts = tonumber(oldest_with_score[2])
    end
else
    current_requests = redis.call("ZCOUNT", key, "(" .. clear_before, "+inf")
    if current_requests < limit then
        current_requests = current_requests + 1
        allowed = true
    end

    local oldest_with_score = redis.call("ZRANGEBYSCORE", key, "(" .. clear_before, "+inf", "WITHSCORES", "LIMIT", 0, 1)
    if #oldest_with_score > 0 then
        oldest_ts = tonumber(oldest_with_score[2])
    end
end

local reset_ms = oldest_ts + window
local remaining = limit - current_requests
if remaining < 0 then
    remaining = 0
end

return {allowed and 1 or 0, remaining, reset_ms - now}
`

type SlidingWindowLimiter struct {
	rdb      redis.RedisClient
	cfg      LimiterConfig
	timeFunc func() time.Time
}

func NewSlidingWindowLimiter(rdb redis.RedisClient, cfg LimiterConfig) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		rdb:      rdb,
		cfg:      cfg,
		timeFunc: time.Now,
	}
}

func GenerateUniqueMember(timestampMs int64) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d-%d", timestampMs, time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", timestampMs, hex.EncodeToString(bytes))
}

func (s *SlidingWindowLimiter) Check(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return s.execute(ctx, key, 0)
}

func (s *SlidingWindowLimiter) Simulate(ctx context.Context, key string) (bool, int, time.Duration, error) {
	return s.execute(ctx, key, 1)
}

func (s *SlidingWindowLimiter) execute(ctx context.Context, key string, dryRun int) (bool, int, time.Duration, error) {
	now := s.timeFunc()
	nowMs := now.UnixMilli()
	windowMs := s.cfg.Window.Milliseconds()

	if windowMs <= 0 {
		return false, 0, 0, fmt.Errorf("window duration must be at least 1 millisecond")
	}

	redisKey := fmt.Sprintf("rl:sliding_window:%s", key)
	member := GenerateUniqueMember(nowMs)

	res, err := s.rdb.Eval(ctx, SlidingWindowLuaScript, []string{redisKey}, nowMs, windowMs, s.cfg.Limit, member, dryRun)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to execute sliding window Lua script: %w", err)
	}

	slice, ok := res.([]interface{})
	if !ok || len(slice) < 3 {
		return false, 0, 0, fmt.Errorf("invalid Lua script response type")
	}

	allowedVal, ok1 := slice[0].(int64)
	remaining64, ok2 := slice[1].(int64)
	retryAfterMs, ok3 := slice[2].(int64)
	if !ok1 || !ok2 || !ok3 {
		return false, 0, 0, fmt.Errorf("failed to parse Lua script elements")
	}

	allowed := allowedVal == 1
	remaining := int(remaining64)
	
	var retryAfter time.Duration
	if retryAfterMs > 0 {
		retryAfter = time.Duration(retryAfterMs) * time.Millisecond
	} else if !allowed {
		retryAfter = s.cfg.Window
	}

	return allowed, remaining, retryAfter, nil
}
