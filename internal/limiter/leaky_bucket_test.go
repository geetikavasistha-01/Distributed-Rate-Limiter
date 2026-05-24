package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	realredis "github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/redis/go-redis/v9"
)

func TestLeakyBucketLimiter(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := LimiterConfig{Limit: 3, Window: 3 * time.Second}
	lim := NewLeakyBucketLimiter(realredis.NewClient(rdb), cfg)
	ctx := context.Background()
	key := "test-leaky"

	// 1st request
	allowed, _, _, err := lim.Check(ctx, key)
	if err != nil || !allowed {
		t.Fatalf("expected 1st request allowed: %v", err)
	}

	// simulate dry run
	allowed, _, _, _ = lim.Simulate(ctx, key)
	if !allowed {
		t.Fatalf("expected dry run request allowed")
	}

	lim.Check(ctx, key)
	lim.Check(ctx, key)

	// Blocked
	allowed, _, _, _ = lim.Check(ctx, key)
	if allowed {
		t.Fatalf("expected 4th request to be blocked")
	}
}
