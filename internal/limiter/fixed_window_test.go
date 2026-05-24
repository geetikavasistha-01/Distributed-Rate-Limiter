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

func TestFixedWindowLimiter_Allow(t *testing.T) {
	// Create in-memory mock Redis server
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	// Instantiate redis client pointing to miniredis
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	client := realredis.NewClient(rdb)
	lim := NewFixedWindowLimiter(client)

	ctx := context.Background()
	key := "test-user-1"
	cfg := LimitConfig{
		Limit:  3,
		Window: 2 * time.Second,
	}

	// 1st request - Allowed
	res, err := lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 1st request to be allowed")
	}
	if res.Remaining != 2 {
		t.Errorf("expected 2 remaining tokens, got %d", res.Remaining)
	}

	// 2nd request - Allowed
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 2nd request to be allowed")
	}
	if res.Remaining != 1 {
		t.Errorf("expected 1 remaining token, got %d", res.Remaining)
	}

	// 3rd request - Allowed
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 3rd request to be allowed")
	}
	if res.Remaining != 0 {
		t.Errorf("expected 0 remaining tokens, got %d", res.Remaining)
	}

	// 4th request - Blocked (limit exceeded)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if res.Allowed {
		t.Error("expected 4th request to be blocked")
	}
	if res.Remaining != 0 {
		t.Errorf("expected 0 remaining tokens when blocked, got %d", res.Remaining)
	}

	// Fast-forward time in miniredis to simulate window expiration
	mr.FastForward(3 * time.Second)

	// 5th request - Allowed (window has reset)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected request to be allowed after window reset")
	}
	if res.Remaining != 2 {
		t.Errorf("expected 2 remaining tokens, got %d", res.Remaining)
	}
}

func TestFixedWindowLimiter_Concurrency(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	client := realredis.NewClient(rdb)
	lim := NewFixedWindowLimiter(client)

	ctx := context.Background()
	key := "concurrent-user"
	cfg := LimitConfig{
		Limit:  20,
		Window: 10 * time.Second,
	}

	var wg sync.WaitGroup
	numRequests := 100
	allowedChan := make(chan bool, numRequests)

	// Launch concurrent requests to verify race-safety and atomic Lua logic
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := lim.Allow(ctx, key, cfg)
			if err == nil {
				allowedChan <- res.Allowed
			} else {
				allowedChan <- false
			}
		}()
	}

	wg.Wait()
	close(allowedChan)

	allowedCount := 0
	blockedCount := 0
	for allowed := range allowedChan {
		if allowed {
			allowedCount++
		} else {
			blockedCount++
		}
	}

	if int64(allowedCount) != cfg.Limit {
		t.Errorf("expected exactly %d requests allowed under concurrency, got %d", cfg.Limit, allowedCount)
	}
	if blockedCount != numRequests-allowedCount {
		t.Errorf("expected %d requests blocked, got %d", numRequests-allowedCount, blockedCount)
	}
}
