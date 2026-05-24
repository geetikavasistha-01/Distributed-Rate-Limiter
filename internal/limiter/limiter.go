package limiter

import (
	"context"
	"time"
)

// LimitConfig configures the rate limit constraints for a specific request scope.
type LimitConfig struct {
	Limit  int64         // Maximum number of requests allowed in a window
	Window time.Duration // Duration of the rate limit window
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
