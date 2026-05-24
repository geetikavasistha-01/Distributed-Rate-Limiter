package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	realredis "github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/redis/go-redis/v9"
)

func TestSlidingWindowLimiter(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := LimiterConfig{Limit: 3, Window: 5 * time.Second}
	lim := NewSlidingWindowLimiter(realredis.NewClient(rdb), cfg)
	ctx := context.Background()
	key := "test-sliding"

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

	// 2nd and 3rd requests
	lim.Check(ctx, key)
	lim.Check(ctx, key)

	// 4th request blocked
	allowed, _, _, _ = lim.Check(ctx, key)
	if allowed {
		t.Fatalf("expected 4th request to be blocked")
	}

	// Fast forward past window
	mr.FastForward(6 * time.Second)
	allowed, _, _, _ = lim.Check(ctx, key)
	if !allowed {
		t.Fatalf("expected request allowed after reset")
	}
}
