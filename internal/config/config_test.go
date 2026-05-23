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
}

func TestConfigEnvOverwrites(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("ENV", "production")
	_ = os.Setenv("SHUTDOWN_TIMEOUT", "10s")

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
}
