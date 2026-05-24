package limiter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	realredis "github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/redis/go-redis/v9"
)

func TestFixedWindowLimiter(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	client := realredis.NewClient(rdb)

	cfg := LimiterConfig{Limit: 3, Window: 2 * time.Second}
	lim := NewFixedWindowLimiter(client, cfg)
	ctx := context.Background()
	key := "test-user-1"

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
	allowed, _, _, err = lim.Check(ctx, key)
	if allowed {
		t.Fatalf("expected 4th request to be blocked")
	}

	// Wait and reset
	mr.FastForward(3 * time.Second)
	allowed, _, _, _ = lim.Check(ctx, key)
	if !allowed {
		t.Fatalf("expected request allowed after reset")
	}
}

func TestFixedWindowLimiter_Concurrency(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	
	cfg := LimiterConfig{Limit: 20, Window: 10 * time.Second}
	lim := NewFixedWindowLimiter(realredis.NewClient(rdb), cfg)
	
	var wg sync.WaitGroup
	numRequests := 100
	allowedChan := make(chan bool, numRequests)
	
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _, err := lim.Check(context.Background(), "concurrent")
			if err == nil {
				allowedChan <- allowed
			} else {
				allowedChan <- false
			}
		}()
	}
	
	wg.Wait()
	close(allowedChan)
	
	allowedCount := 0
	for allowed := range allowedChan {
		if allowed {
			allowedCount++
		}
	}
	if allowedCount != 20 {
		t.Errorf("expected 20 allowed, got %d", allowedCount)
	}
}
