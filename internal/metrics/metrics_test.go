package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_ObserveRequest(t *testing.T) {
	ObserveRequest("fixed_window", "allowed", time.Millisecond*5)
	
	reqCount := testutil.ToFloat64(RequestsTotal.WithLabelValues("fixed_window", "allowed"))
	if reqCount != 1.0 {
		t.Errorf("expected 1.0 requests, got %v", reqCount)
	}

	ObserveRedisError("fixed_window")
	errCount := testutil.ToFloat64(RedisErrorsTotal.WithLabelValues("fixed_window"))
	if errCount != 1.0 {
		t.Errorf("expected 1.0 error, got %v", errCount)
	}
}

func TestHotKeyTracker(t *testing.T) {
	tracker := NewHotKeyTracker(3)
	
	tracker.Record("key1")
	tracker.Record("key1")
	tracker.Record("key1")
	tracker.Record("key2")
	
	if !tracker.IsHot("key1") {
		t.Error("expected key1 to be hot")
	}
	if tracker.IsHot("key2") {
		t.Error("expected key2 to not be hot")
	}
	
	top := tracker.TopKeys(1)
	if len(top) != 1 || top[0] != "key1" {
		t.Errorf("expected key1 to be top key, got %v", top)
	}
}
