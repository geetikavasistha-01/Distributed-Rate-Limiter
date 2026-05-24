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

func TestTokenBucketLimiter_Allow(t *testing.T) {
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
	lim := NewTokenBucketLimiter(client)

	// Mock the clock for deterministic testing
	mockTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	lim.timeFunc = func() time.Time {
		return mockTime
	}

	ctx := context.Background()
	key := "test-user-token"
	cfg := LimitConfig{
		Limit:  3,
		Window: 3 * time.Second, // Refill rate: 3 tokens per 3 seconds = 1 token per second (0.001 tokens/ms)
	}

	// Request 1: Allowed (3 -> 2 tokens remaining)
	res, err := lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 1st request to be allowed")
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining=2, got %d", res.Remaining)
	}

	// Request 2: Allowed (2 -> 1 token remaining)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 2nd request to be allowed")
	}

	// Request 3: Allowed (1 -> 0 tokens remaining)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 3rd request to be allowed")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining=0, got %d", res.Remaining)
	}

	// Request 4: Blocked immediately (0 tokens remaining)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if res.Allowed {
		t.Error("expected 4th request to be blocked")
	}

	// Check ResetTime: Time when bucket is fully refilled (3 seconds from mockTime)
	expectedReset := mockTime.Add(3 * time.Second)
	if !res.ResetTime.Equal(expectedReset) {
		t.Errorf("expected ResetTime exactly %v, got %v", expectedReset, res.ResetTime)
	}

	// Fast-forward 1 second (should refill exactly 1 token)
	mockTime = mockTime.Add(1 * time.Second)
	mr.FastForward(1 * time.Second)

	// Request 5: Allowed (consumes refilled 1 token, remaining drops to 0)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected request to be allowed after 1 token refilled")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining=0 after consuming refilled token, got %d", res.Remaining)
	}

	// Fast-forward 5 seconds (should refill up to capacity limit of 3 tokens)
	mockTime = mockTime.Add(5 * time.Second)
	mr.FastForward(5 * time.Second)

	// Request 6: Allowed (consumes from fully refilled bucket, remaining drops to 2)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected request to be allowed after full refill")
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining=2, got %d", res.Remaining)
	}
}

func TestTokenBucketLimiter_Concurrency(t *testing.T) {
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
	lim := NewTokenBucketLimiter(client)

	ctx := context.Background()
	key := "concurrent-token-user"
	cfg := LimitConfig{
		Limit:  10,
		Window: 10 * time.Second,
	}

	var wg sync.WaitGroup
	numRequests := 100
	allowedChan := make(chan bool, numRequests)

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
