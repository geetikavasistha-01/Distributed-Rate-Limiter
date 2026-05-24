package api

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// MockRedisClient is a mock implementing the RedisClient interface for testing handlers.
type MockRedisClient struct {
	PingFunc             func(ctx context.Context) error
	CloseFunc            func() error
	EvalFunc             func(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	EvalShaFunc          func(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error)
	ScriptLoadFunc       func(ctx context.Context, script string) (string, error)
	IncrByFunc           func(ctx context.Context, key string, value int64) (int64, error)
	ExpireFunc           func(ctx context.Context, key string, expiration time.Duration) (bool, error)
	TTLFunc              func(ctx context.Context, key string) (time.Duration, error)
	ZAddFunc             func(ctx context.Context, key string, members ...redis.Z) (int64, error)
	ZRemRangeByScoreFunc func(ctx context.Context, key, min, max string) (int64, error)
	ZCardFunc            func(ctx context.Context, key string) (int64, error)
}

func (m *MockRedisClient) Ping(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

func (m *MockRedisClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockRedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	if m.EvalFunc != nil {
		return m.EvalFunc(ctx, script, keys, args...)
	}
	return nil, nil
}

func (m *MockRedisClient) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error) {
	if m.EvalShaFunc != nil {
		return m.EvalShaFunc(ctx, sha1, keys, args...)
	}
	return nil, nil
}

func (m *MockRedisClient) ScriptLoad(ctx context.Context, script string) (string, error) {
	if m.ScriptLoadFunc != nil {
		return m.ScriptLoadFunc(ctx, script)
	}
	return "", nil
}

func (m *MockRedisClient) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	if m.IncrByFunc != nil {
		return m.IncrByFunc(ctx, key, value)
	}
	return 0, nil
}

func (m *MockRedisClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if m.ExpireFunc != nil {
		return m.ExpireFunc(ctx, key, expiration)
	}
	return true, nil
}

func (m *MockRedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	if m.TTLFunc != nil {
		return m.TTLFunc(ctx, key)
	}
	return 0, nil
}

func (m *MockRedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) (int64, error) {
	if m.ZAddFunc != nil {
		return m.ZAddFunc(ctx, key, members...)
	}
	return 0, nil
}

func (m *MockRedisClient) ZRemRangeByScore(ctx context.Context, key, min, max string) (int64, error) {
	if m.ZRemRangeByScoreFunc != nil {
		return m.ZRemRangeByScoreFunc(ctx, key, min, max)
	}
	return 0, nil
}

func (m *MockRedisClient) ZCard(ctx context.Context, key string) (int64, error) {
	if m.ZCardFunc != nil {
		return m.ZCardFunc(ctx, key)
	}
	return 0, nil
}
