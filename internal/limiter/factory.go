package limiter

import (
	"fmt"
)

// New constructs a Limiter instance based on the provided configuration's algorithm.
func New(cfg Config, redisClient RedisClient) (Limiter, error) {
	switch cfg.Algorithm {
	case "fixed_window":
		return NewFixedWindowLimiter(redisClient), nil
	case "sliding_window":
		return NewSlidingWindowLimiter(redisClient), nil
	case "token_bucket":
		return NewTokenBucketLimiter(redisClient), nil
	case "leaky_bucket":
		return NewLeakyBucketLimiter(redisClient), nil
	default:
		return nil, fmt.Errorf("unknown algorithm: %s", cfg.Algorithm)
	}
}
