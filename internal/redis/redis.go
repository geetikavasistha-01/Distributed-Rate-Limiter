package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/config"
	"github.com/redis/go-redis/v9"
)

// RedisClient defines the subset of Redis commands needed for rate limiting.
// Decoupling via this interface facilitates clean unit-testing and mockability.
type RedisClient interface {
	Ping(ctx context.Context) error
	Close() error
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error)
	ScriptLoad(ctx context.Context, script string) (string, error)
	IncrBy(ctx context.Context, key string, value int64) (int64, error)
	Expire(ctx context.Context, key string, expiration time.Duration) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	ZAdd(ctx context.Context, key string, members ...redis.Z) (int64, error)
	ZRemRangeByScore(ctx context.Context, key, min, max string) (int64, error)
	ZCard(ctx context.Context, key string) (int64, error)
}

// Client implements RedisClient using a concrete go-redis connection pool.
type Client struct {
	rdb *redis.Client
}

// NewClient creates a new Client wrapper.
func NewClient(rdb *redis.Client) *Client {
	return &Client{rdb: rdb}
}

// ConnectWithRetry initializes a Redis connection pool and validates connection availability.
// Retries on startup failures using an exponential backoff strategy to account for container startup delays.
func ConnectWithRetry(cfg *config.Config) (RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     cfg.RedisPoolSize,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	})

	client := NewClient(rdb)

	maxRetries := 5
	backoff := 500 * time.Millisecond

	var err error
	for i := 1; i <= maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.RedisDialTimeout)
		err = client.Ping(ctx)
		cancel()

		if err == nil {
			slog.Info("successfully established Redis connection pool", slog.String("addr", cfg.RedisAddr))
			return client, nil
		}

		slog.Warn("failed to connect to Redis, retrying...",
			slog.Int("attempt", i),
			slog.Int("max_attempts", maxRetries),
			slog.String("backoff", backoff.String()),
			slog.Any("error", err),
		)

		if i < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}

	return nil, fmt.Errorf("could not establish Redis connection after %d attempts: %w", maxRetries, err)
}

// Ping evaluates if the Redis server is responsive.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close closes the Redis connection pool cleanly.
func (c *Client) Close() error {
	slog.Info("closing Redis client connection pool")
	return c.rdb.Close()
}

// Eval executes a Lua script within the database.
func (c *Client) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return c.rdb.Eval(ctx, script, keys, args...).Result()
}

// EvalSha executes a cached Lua script using its SHA1 signature.
func (c *Client) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error) {
	return c.rdb.EvalSha(ctx, sha1, keys, args...).Result()
}

// ScriptLoad loads a script to the database scripts cache.
func (c *Client) ScriptLoad(ctx context.Context, script string) (string, error) {
	return c.rdb.ScriptLoad(ctx, script).Result()
}

// IncrBy increments a key value by a given number.
func (c *Client) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.rdb.IncrBy(ctx, key, value).Result()
}

// Expire sets a time-to-live timeout constraint on a key.
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.rdb.Expire(ctx, key, expiration).Result()
}

// TTL fetches the remaining time-to-live for a key.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.rdb.TTL(ctx, key).Result()
}

// ZAdd adds members with scores to a sorted set.
func (c *Client) ZAdd(ctx context.Context, key string, members ...redis.Z) (int64, error) {
	return c.rdb.ZAdd(ctx, key, members...).Result()
}

// ZRemRangeByScore deletes sorted set members based on a range of scores.
func (c *Client) ZRemRangeByScore(ctx context.Context, key, min, max string) (int64, error) {
	return c.rdb.ZRemRangeByScore(ctx, key, min, max).Result()
}

// ZCard returns the cardinality of a sorted set.
func (c *Client) ZCard(ctx context.Context, key string) (int64, error) {
	return c.rdb.ZCard(ctx, key).Result()
}
