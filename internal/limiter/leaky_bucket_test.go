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

func TestLeakyBucketLimiter_Allow(t *testing.T) {
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
	lim := NewLeakyBucketLimiter(client)

	// Mock the clock for deterministic testing
	mockTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	lim.timeFunc = func() time.Time {
		return mockTime
	}

	ctx := context.Background()
	key := "test-user-leaky"
	cfg := LimitConfig{
		Limit:  3,
		Window: 3 * time.Second, // Emission interval: 1s per request. Delay tolerance: 3s.
	}

	// Request 1: Allowed (Remaining quota: 2. Reset time is now + 1s = 12:00:01)
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

	// Request 2: Allowed (Immediate burst. Remaining quota: 1. Reset time is 12:00:02)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 2nd request to be allowed")
	}
	if res.Remaining != 1 {
		t.Errorf("expected remaining=1, got %d", res.Remaining)
	}

	// Request 3: Allowed (Immediate burst. Remaining quota: 0. Reset time is 12:00:03)
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

	// Request 4: Blocked (Immediate burst exceeds capacity/delay tolerance buffer)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if res.Allowed {
		t.Error("expected 4th request to be blocked")
	}

	// Check ResetTime: Virtual queue becomes completely empty at 12:00:03
	expectedReset := mockTime.Add(3 * time.Second)
	if !res.ResetTime.Equal(expectedReset) {
		t.Errorf("expected ResetTime exactly %v, got %v", expectedReset, res.ResetTime)
	}

	// Fast-forward 1.5 seconds (queue should leak/drain 1 slot)
	mockTime = mockTime.Add(1500 * time.Millisecond)
	mr.FastForward(1500 * time.Millisecond)

	// Request 5: Allowed (as 1 slot leaked out)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected request to be allowed after queue leak")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining=0 after consuming leaked slot, got %d", res.Remaining)
	}

	// Fast-forward 10 seconds (queue should completely empty/drain)
	mockTime = mockTime.Add(10 * time.Second)
	mr.FastForward(10 * time.Second)

	// Request 6: Allowed (consumes from empty queue, remaining = 2)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected request to be allowed after complete queue drain")
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining=2, got %d", res.Remaining)
	}
}

func TestLeakyBucketLimiter_Concurrency(t *testing.T) {
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
	lim := NewLeakyBucketLimiter(client)

	ctx := context.Background()
	key := "concurrent-leaky-user"
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

func TestLeakyBucketLimiter_DryRun(t *testing.T) {
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
	lim := NewLeakyBucketLimiter(client)

	ctx := context.Background()
	key := "test-dryrun-leaky"
	cfgDry := LimitConfig{
		Limit:  3,
		Window: 3 * time.Second,
		DryRun: true,
	}
	cfgMut := LimitConfig{
		Limit:  3,
		Window: 3 * time.Second,
		DryRun: false,
	}

	// 1. Dry run evaluation on empty database
	res, err := lim.Allow(ctx, key, cfgDry)
	if err != nil {
		t.Fatalf("DryRun Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected dry-run request to be allowed")
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining=2, got %d", res.Remaining)
	}
	if len(mr.Keys()) > 0 {
		t.Errorf("expected no keys in redis, but found: %v", mr.Keys())
	}

	// 2. Mutating evaluation
	res, err = lim.Allow(ctx, key, cfgMut)
	if err != nil {
		t.Fatalf("Mutating Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected mutating request to be allowed")
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining=2, got %d", res.Remaining)
	}
	if len(mr.Keys()) == 0 {
		t.Error("expected key to be written to redis")
	}

	// Consume more to hit limit
	_, _ = lim.Allow(ctx, key, cfgMut)
	_, _ = lim.Allow(ctx, key, cfgMut)

	// 3. Dry run at limit
	res, err = lim.Allow(ctx, key, cfgDry)
	if err != nil {
		t.Fatalf("DryRun Allow failed: %v", err)
	}
	if res.Allowed {
		t.Error("expected dry-run request to be blocked at the limit")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining=0, got %d", res.Remaining)
	}
}
