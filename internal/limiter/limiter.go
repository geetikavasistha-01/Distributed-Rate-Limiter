package limiter

import (
	"context"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// LimiterConfig configures the rate limit constraints for a specific request scope.
type LimiterConfig struct {
	Algorithm string        // e.g. "fixed_window", "sliding_window"
	Limit     int64         // Maximum number of requests allowed in a window
	Window    time.Duration // Duration of the rate limit window
}

// RedisClient is an alias for the storage layer's Redis client interface.
type RedisClient = redis.RedisClient

// Limiter specifies the interface for rate limiting algorithms.
type Limiter interface {
	// Check evaluates if a request identified by key is allowed.
	// It consumes the limit quota if allowed.
	Check(ctx context.Context, key string) (allowed bool, remaining int, retryAfter time.Duration, err error)

	// Simulate evaluates if a request would be allowed without consuming the quota.
	Simulate(ctx context.Context, key string) (allowed bool, remaining int, retryAfter time.Duration, err error)
}
