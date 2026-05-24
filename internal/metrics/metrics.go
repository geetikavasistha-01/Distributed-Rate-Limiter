package metrics

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limiter_requests_total",
			Help: "Total number of rate limiter requests evaluated.",
		},
		[]string{"algorithm", "status", "key_type"},
	)

	EvaluationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rate_limiter_duration_seconds",
			Help:    "Latency of rate limiter evaluation in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"algorithm", "status"},
	)

	RedisDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rate_limiter_redis_duration_seconds",
			Help:    "Latency of Redis operations in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	HotKeyHits = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rate_limiter_hot_key_hits",
			Help: "Hits count of the hottest keys in the rate limiter.",
		},
		[]string{"key"},
	)
)

// Init registers Prometheus metrics collectors exactly once.
func Init() {
	once.Do(func() {
		prometheus.MustRegister(RequestsTotal)
		prometheus.MustRegister(EvaluationDuration)
		prometheus.MustRegister(RedisDuration)
		prometheus.MustRegister(HotKeyHits)
	})
}

// UpdateHotKeys queries the top 10 keys from Redis ZSET rl:hotkeys and updates the Prometheus gauge.
func UpdateHotKeys(ctx context.Context, rdb redis.RedisClient) {
	if rdb == nil {
		return
	}

	// Lua script to fetch top 10 keys with scores from rl:hotkeys ZSET (highest hits first)
	const fetchLua = "return redis.call('ZREVRANGE', KEYS[1], 0, 9, 'WITHSCORES')"
	res, err := rdb.Eval(ctx, fetchLua, []string{"rl:hotkeys"})
	if err != nil {
		slog.Error("failed to fetch hot keys from redis", slog.Any("error", err))
		return
	}

	slice, ok := res.([]interface{})
	if !ok {
		return
	}

	// Reset gauge to remove keys that are no longer in the top 10
	HotKeyHits.Reset()

	for i := 0; i < len(slice); i += 2 {
		if i+1 >= len(slice) {
			break
		}
		key, ok1 := slice[i].(string)
		scoreStr, ok2 := slice[i+1].(string)
		if ok1 && ok2 {
			score, err := strconv.ParseFloat(scoreStr, 64)
			if err == nil {
				HotKeyHits.WithLabelValues(key).Set(score)
			}
		}
	}
}

// IncrementHotKey increments the request frequency count of a key inside Redis.
func IncrementHotKey(ctx context.Context, rdb redis.RedisClient, key string) {
	if rdb == nil || key == "" {
		return
	}
	// Atomic ZINCRBY and EXPIRE to auto-clean up after 24h of inactivity
	const incrLua = "redis.call('ZINCRBY', KEYS[1], 1, ARGV[1]); redis.call('EXPIRE', KEYS[1], ARGV[2])"
	_, _ = rdb.Eval(ctx, incrLua, []string{"rl:hotkeys"}, key, 86400)
}

// GetKeyType parses the key string to extract its prefix/type (e.g. "ip" or "user"), defaulting to "default".
func GetKeyType(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i]
		}
	}
	return "default"
}
