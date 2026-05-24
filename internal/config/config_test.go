package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	// Clear environment variables that might interfere with tests
	os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected PORT=8080, got %d", cfg.Port)
	}
	if cfg.Env != "development" {
		t.Errorf("expected ENV=development, got %s", cfg.Env)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("expected SHUTDOWN_TIMEOUT=5s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("expected READ_TIMEOUT=5s, got %v", cfg.ReadTimeout)
	}
	if cfg.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("expected READ_HEADER_TIMEOUT=2s, got %v", cfg.ReadHeaderTimeout)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("expected WRITE_TIMEOUT=10s, got %v", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 120*time.Second {
		t.Errorf("expected IDLE_TIMEOUT=120s, got %v", cfg.IdleTimeout)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("expected REDIS_ADDR=localhost:6379, got %s", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "" {
		t.Errorf("expected REDIS_PASSWORD=empty, got %s", cfg.RedisPassword)
	}
	if cfg.RedisDB != 0 {
		t.Errorf("expected REDIS_DB=0, got %d", cfg.RedisDB)
	}
	if cfg.RedisPoolSize != 10 {
		t.Errorf("expected REDIS_POOL_SIZE=10, got %d", cfg.RedisPoolSize)
	}
	if cfg.RedisDialTimeout != 5*time.Second {
		t.Errorf("expected REDIS_DIAL_TIMEOUT=5s, got %v", cfg.RedisDialTimeout)
	}
	if cfg.RedisReadTimeout != 3*time.Second {
		t.Errorf("expected REDIS_READ_TIMEOUT=3s, got %v", cfg.RedisReadTimeout)
	}
	if cfg.RedisWriteTimeout != 3*time.Second {
		t.Errorf("expected REDIS_WRITE_TIMEOUT=3s, got %v", cfg.RedisWriteTimeout)
	}
}

func TestConfigEnvOverwrites(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("ENV", "production")
	_ = os.Setenv("SHUTDOWN_TIMEOUT", "10s")
	_ = os.Setenv("REDIS_ADDR", "redis-host:6379")
	_ = os.Setenv("REDIS_PASSWORD", "secret")
	_ = os.Setenv("REDIS_DB", "2")
	_ = os.Setenv("REDIS_POOL_SIZE", "50")
	_ = os.Setenv("REDIS_DIAL_TIMEOUT", "2s")
	_ = os.Setenv("REDIS_READ_TIMEOUT", "1s")
	_ = os.Setenv("REDIS_WRITE_TIMEOUT", "1s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected PORT=9090, got %d", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Errorf("expected ENV=production, got %s", cfg.Env)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("expected SHUTDOWN_TIMEOUT=10s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.RedisAddr != "redis-host:6379" {
		t.Errorf("expected REDIS_ADDR=redis-host:6379, got %s", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "secret" {
		t.Errorf("expected REDIS_PASSWORD=secret, got %s", cfg.RedisPassword)
	}
	if cfg.RedisDB != 2 {
		t.Errorf("expected REDIS_DB=2, got %d", cfg.RedisDB)
	}
	if cfg.RedisPoolSize != 50 {
		t.Errorf("expected REDIS_POOL_SIZE=50, got %d", cfg.RedisPoolSize)
	}
	if cfg.RedisDialTimeout != 2*time.Second {
		t.Errorf("expected REDIS_DIAL_TIMEOUT=2s, got %v", cfg.RedisDialTimeout)
	}
	if cfg.RedisReadTimeout != 1*time.Second {
		t.Errorf("expected REDIS_READ_TIMEOUT=1s, got %v", cfg.RedisReadTimeout)
	}
	if cfg.RedisWriteTimeout != 1*time.Second {
		t.Errorf("expected REDIS_WRITE_TIMEOUT=1s, got %v", cfg.RedisWriteTimeout)
	}
}

func TestConfigInvalidValues(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("PORT", "invalid-port")

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid PORT, got nil")
	}

	os.Clearenv()
	_ = os.Setenv("SHUTDOWN_TIMEOUT", "invalid-duration")

	_, err = Load()
	if err == nil {
		t.Error("expected error for invalid SHUTDOWN_TIMEOUT, got nil")
	}

	os.Clearenv()
	_ = os.Setenv("REDIS_DB", "invalid-db")
	_, err = Load()
	if err == nil {
		t.Error("expected error for invalid REDIS_DB, got nil")
	}

	os.Clearenv()
	_ = os.Setenv("REDIS_POOL_SIZE", "invalid-pool-size")
	_, err = Load()
	if err == nil {
		t.Error("expected error for invalid REDIS_POOL_SIZE, got nil")
	}

	os.Clearenv()
	_ = os.Setenv("REDIS_DIAL_TIMEOUT", "invalid-duration")
	_, err = Load()
	if err == nil {
		t.Error("expected error for invalid REDIS_DIAL_TIMEOUT, got nil")
	}
}
