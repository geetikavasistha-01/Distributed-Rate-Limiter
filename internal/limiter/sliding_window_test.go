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

func TestSlidingWindowLimiter_Allow(t *testing.T) {
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
	lim := NewSlidingWindowLimiter(client)

	// Mock the clock provider for deterministic testing
	mockTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	lim.timeFunc = func() time.Time {
		return mockTime
	}

	ctx := context.Background()
	key := "test-user-sliding"
	cfg := LimitConfig{
		Limit:  3,
		Window: 5 * time.Second,
	}

	// Request 1: Allow
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
	firstRequestTime := mockTime

	// Wait 1 second (mocked) and request 2: Allow
	mockTime = mockTime.Add(1 * time.Second)
	mr.FastForward(1 * time.Second)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected 2nd request to be allowed")
	}

	// Wait 1 second (mocked) and request 3: Allow
	mockTime = mockTime.Add(1 * time.Second)
	mr.FastForward(1 * time.Second)
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

	// Request 4: Blocked immediately
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if res.Allowed {
		t.Error("expected 4th request to be blocked")
	}

	// Check ResetTime: Should be exactly 5 seconds after the FIRST request timestamp
	expectedReset := firstRequestTime.Add(5 * time.Second)
	if !res.ResetTime.Equal(expectedReset) {
		t.Errorf("expected ResetTime exactly %v, got %v", expectedReset, res.ResetTime)
	}

	// Fast-forward 4 seconds (total 6 seconds since Request 1, so Request 1 falls out of the sliding log)
	mockTime = mockTime.Add(4 * time.Second)
	mr.FastForward(4 * time.Second)

	// Request 5: Allowed (as Request 1 expired, slot 1 freed up)
	res, err = lim.Allow(ctx, key, cfg)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed {
		t.Error("expected request to be allowed after first log item slid out")
	}
	if res.Remaining != 1 {
		// Timestamps in sliding log: 12:00:02 (Req 3), 12:00:06 (Req 5).
		// Req 1 (12:00:00) and Req 2 (12:00:01) have expired because ZREMRANGEBYSCORE is inclusive.
		// So remaining = 3 - 2 = 1.
		t.Errorf("expected remaining=1 after sliding reset request, got %d", res.Remaining)
	}
}

func TestSlidingWindowLimiter_Concurrency(t *testing.T) {
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
	lim := NewSlidingWindowLimiter(client)

	ctx := context.Background()
	key := "concurrent-sliding-user"
	cfg := LimitConfig{
		Limit:  15,
		Window: 10 * time.Second,
	}

	var wg sync.WaitGroup
	numRequests := 80
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
