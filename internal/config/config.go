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
	ServerPort          int
	Env                 string
	LogLevel            string
	ShutdownTimeout     time.Duration
	ReadTimeout         time.Duration
	ReadHeaderTimeout   time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	RedisURL            string
	RedisPassword       string
	RedisDB             int
	RedisPoolSize       int
	RedisDialTimeout    time.Duration
	RedisReadTimeout    time.Duration
	RedisWriteTimeout   time.Duration
	RateLimitAlgorithm  string
	RateLimitRequests   int64
	RateLimitWindow     time.Duration
}

// Load loads the configuration from environment variables.
func Load() (*Config, error) {
	_ = LoadDotEnv(".env")

	port, err := getEnvInt("SERVER_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}

	limit, err := getEnvInt64("RATE_LIMIT_REQUESTS", 100)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_REQUESTS: %w", err)
	}

	window, err := getEnvDuration("RATE_LIMIT_WINDOW", time.Minute)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_WINDOW: %w", err)
	}

	return &Config{
		ServerPort:          port,
		Env:                 getEnvString("ENV", "development"),
		LogLevel:            getEnvString("LOG_LEVEL", "info"),
		ShutdownTimeout:     getEnvDurationSilent("SHUTDOWN_TIMEOUT", 5*time.Second),
		ReadTimeout:         getEnvDurationSilent("READ_TIMEOUT", 5*time.Second),
		ReadHeaderTimeout:   getEnvDurationSilent("READ_HEADER_TIMEOUT", 2*time.Second),
		WriteTimeout:        getEnvDurationSilent("WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:         getEnvDurationSilent("IDLE_TIMEOUT", 120*time.Second),
		RedisURL:            getEnvString("REDIS_URL", "localhost:6379"),
		RedisPassword:       getEnvString("REDIS_PASSWORD", ""),
		RedisDB:             getEnvIntSilent("REDIS_DB", 0),
		RedisPoolSize:       getEnvIntSilent("REDIS_POOL_SIZE", 10),
		RedisDialTimeout:    getEnvDurationSilent("REDIS_DIAL_TIMEOUT", 5*time.Second),
		RedisReadTimeout:    getEnvDurationSilent("REDIS_READ_TIMEOUT", 3*time.Second),
		RedisWriteTimeout:   getEnvDurationSilent("REDIS_WRITE_TIMEOUT", 3*time.Second),
		RateLimitAlgorithm:  getEnvString("RATE_LIMIT_ALGORITHM", "fixed_window"),
		RateLimitRequests:   limit,
		RateLimitWindow:     window,
	}, nil
}

// LoadDotEnv parses a standard key=value .env file.
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

			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}

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

func getEnvIntSilent(key string, defaultValue int) int {
	val, err := getEnvInt(key, defaultValue)
	if err != nil {
		return defaultValue
	}
	return val
}

func getEnvInt64(key string, defaultValue int64) (int64, error) {
	valueStr, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue, nil
	}
	val, err := strconv.ParseInt(valueStr, 10, 64)
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

func getEnvDurationSilent(key string, defaultValue time.Duration) time.Duration {
	d, err := getEnvDuration(key, defaultValue)
	if err != nil {
		return defaultValue
	}
	return d
}
