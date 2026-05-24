package limiter_test

import (
	"testing"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
)

func TestFactoryFixedWindow(t *testing.T) {
	cfg := limiter.LimiterConfig{Algorithm: "fixed_window"}
	l, err := limiter.New(cfg, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if l == nil {
		t.Fatalf("expected non-nil limiter")
	}
}

func TestFactoryslidingWindow(t *testing.T) {
	cfg := limiter.LimiterConfig{Algorithm: "sliding_window"}
	l, err := limiter.New(cfg, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if l == nil {
		t.Fatalf("expected non-nil limiter")
	}
}

func TestFactoryTokenBucket(t *testing.T) {
	cfg := limiter.LimiterConfig{Algorithm: "token_bucket"}
	l, err := limiter.New(cfg, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if l == nil {
		t.Fatalf("expected non-nil limiter")
	}
}

func TestFactoryLeakyBucket(t *testing.T) {
	cfg := limiter.LimiterConfig{Algorithm: "leaky_bucket"}
	l, err := limiter.New(cfg, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if l == nil {
		t.Fatalf("expected non-nil limiter")
	}
}

func TestFactoryUnknown(t *testing.T) {
	cfg := limiter.LimiterConfig{Algorithm: "unknown"}
	l, err := limiter.New(cfg, nil)
	if err == nil {
		t.Fatalf("expected error for unknown algorithm, got nil")
	}
	if l != nil {
		t.Fatalf("expected nil limiter, got %v", l)
	}
}
