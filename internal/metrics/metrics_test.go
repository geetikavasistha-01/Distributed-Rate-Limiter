package metrics

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	realredis "github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
)

func TestMetrics_HotKeys(t *testing.T) {
	// Initialize metrics registration
	Init()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdbClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdbClient.Close()

	rdb := realredis.NewClient(rdbClient)
	ctx := context.Background()

	// 1. Increment a hotkey multiple times
	IncrementHotKey(ctx, rdb, "key1")
	IncrementHotKey(ctx, rdb, "key1")
	IncrementHotKey(ctx, rdb, "key2")

	// 2. Run UpdateHotKeys to fetch and update gauges
	UpdateHotKeys(ctx, rdb)

	// 3. Verify gauge values using prometheus/testutil
	key1Val := testutil.ToFloat64(HotKeyHits.WithLabelValues("key1"))
	if key1Val != 2.0 {
		t.Errorf("expected key1 metric value to be 2.0, got %f", key1Val)
	}

	key2Val := testutil.ToFloat64(HotKeyHits.WithLabelValues("key2"))
	if key2Val != 1.0 {
		t.Errorf("expected key2 metric value to be 1.0, got %f", key2Val)
	}

	// 4. Test Resetting gauge when keys fall out
	mr.Del("rl:hotkeys")
	UpdateHotKeys(ctx, rdb)

	key1Val = testutil.ToFloat64(HotKeyHits.WithLabelValues("key1"))
	if key1Val != 0.0 {
		t.Errorf("expected key1 metric value to be reset to 0, got %f", key1Val)
	}
}
