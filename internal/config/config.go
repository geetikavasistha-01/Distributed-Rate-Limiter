package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration parameters for the Rate Limiter Service.
type Config struct {
	Port              int
	Env               string
	ShutdownTimeout   time.Duration
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	RedisAddr         string
	RedisPassword     string
}

// Load loads the configuration from environment variables.
// If a .env file exists, it will parse it and load values into the environment
// for keys that are not already set.
func Load() (*Config, error) {
	// Attempt to load from .env file if present
	_ = LoadDotEnv(".env")

	port, err := getEnvInt("PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	shutdownTimeout, err := getEnvDuration("SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}

	readTimeout, err := getEnvDuration("READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid READ_TIMEOUT: %w", err)
	}

	readHeaderTimeout, err := getEnvDuration("READ_HEADER_TIMEOUT", 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid READ_HEADER_TIMEOUT: %w", err)
	}

	writeTimeout, err := getEnvDuration("WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid WRITE_TIMEOUT: %w", err)
	}

	idleTimeout, err := getEnvDuration("IDLE_TIMEOUT", 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid IDLE_TIMEOUT: %w", err)
	}

	return &Config{
		Port:              port,
		Env:               getEnvString("ENV", "development"),
		ShutdownTimeout:   shutdownTimeout,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		RedisAddr:         getEnvString("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnvString("REDIS_PASSWORD", ""),
	}, nil
}

// LoadDotEnv parses a standard key=value .env file and sets environment variables
// if they are not already set in the current process.
func LoadDotEnv(filenames ...string) error {
	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			continue // skip missing files silently
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Strip surrounding quotes if present
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}

			// Only set the env variable if it is not already set by the environment
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, value)
			}
		}
	}
	return nil
}

func getEnvString(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) (int, error) {
	valueStr, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue, nil
	}
	val, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func getEnvDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	valueStr, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(valueStr)
	if err != nil {
		return 0, err
	}
	return d, nil
}
