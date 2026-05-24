package limiter

import (
	"context"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// Config defines the configuration for rate limiting algorithm selection.
type Config struct {
	Algorithm string
}

// RedisClient is an alias for the storage layer's Redis client interface.
type RedisClient = redis.RedisClient

// LimitConfig configures the rate limit constraints for a specific request scope.
type LimitConfig struct {
	Limit  int64         // Maximum number of requests allowed in a window
	Window time.Duration // Duration of the rate limit window
	DryRun bool          // Evaluate rate limit without consuming quota
}

// Result holds the outcome of a rate check request.
type Result struct {
	Allowed   bool          // True if the request is permitted, false if rate limited
	Remaining int64         // Number of requests remaining in the current window
	ResetTime time.Time     // Point in time when the rate limit resets
}

// Limiter specifies the interface for rate limiting algorithms.
type Limiter interface {
	// Allow evaluates if a request identified by key is allowed according to the LimitConfig.
	Allow(ctx context.Context, key string, cfg LimitConfig) (*Result, error)
}
