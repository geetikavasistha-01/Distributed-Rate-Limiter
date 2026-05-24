package config

import (
	"os"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.ServerPort != 8080 {
		t.Errorf("expected default ServerPort to be 8080, got %d", cfg.ServerPort)
	}
	if cfg.Env != "development" {
		t.Errorf("expected ENV=development, got %s", cfg.Env)
	}
	if cfg.RedisURL != "localhost:6379" {
		t.Errorf("expected default REDIS_URL to be localhost:6379, got %s", cfg.RedisURL)
	}
	if cfg.RateLimitAlgorithm != "fixed_window" {
		t.Errorf("expected default RATE_LIMIT_ALGORITHM to be fixed_window, got %s", cfg.RateLimitAlgorithm)
	}
}

func TestConfigEnvOverwrites(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("REDIS_URL", "redis-host:6379")
	os.Setenv("RATE_LIMIT_ALGORITHM", "token_bucket")
	os.Setenv("RATE_LIMIT_REQUESTS", "50")
	
	defer os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.ServerPort != 9090 {
		t.Errorf("expected SERVER_PORT=9090, got %d", cfg.ServerPort)
	}
	if cfg.RedisURL != "redis-host:6379" {
		t.Errorf("expected REDIS_URL=redis-host:6379, got %s", cfg.RedisURL)
	}
	if cfg.RateLimitAlgorithm != "token_bucket" {
		t.Errorf("expected RATE_LIMIT_ALGORITHM=token_bucket, got %s", cfg.RateLimitAlgorithm)
	}
	if cfg.RateLimitRequests != 50 {
		t.Errorf("expected RATE_LIMIT_REQUESTS=50, got %d", cfg.RateLimitRequests)
	}
}

func TestConfigInvalidValues(t *testing.T) {
	os.Setenv("SERVER_PORT", "invalid")
	defer os.Clearenv()

	_, err := Load()
	if err == nil {
		t.Errorf("expected error for invalid SERVER_PORT, got nil")
	}
	
	os.Setenv("SERVER_PORT", "8080")
	os.Setenv("RATE_LIMIT_REQUESTS", "invalid")
	_, err = Load()
	if err == nil {
		t.Errorf("expected error for invalid RATE_LIMIT_REQUESTS, got nil")
	}
	
	os.Setenv("RATE_LIMIT_REQUESTS", "100")
	os.Setenv("RATE_LIMIT_WINDOW", "invalid")
	_, err = Load()
	if err == nil {
		t.Errorf("expected error for invalid RATE_LIMIT_WINDOW, got nil")
	}
}
