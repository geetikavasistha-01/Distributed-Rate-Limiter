package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limiter_requests_total",
			Help: "Total number of rate limit requests processed",
		},
		[]string{"algorithm", "result"}, // result can be "allowed", "rejected"
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rate_limiter_request_duration_seconds",
			Help:    "Duration of rate limit request evaluation",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"algorithm", "result"},
	)

	RedisErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limiter_redis_errors_total",
			Help: "Total number of Redis errors encountered during evaluation",
		},
		[]string{"algorithm"},
	)
)

// ObserveRequest records the metrics for a single rate limit check.
func ObserveRequest(algorithm, status string, duration time.Duration) {
	RequestsTotal.WithLabelValues(algorithm, status).Inc()
	RequestDuration.WithLabelValues(algorithm, status).Observe(duration.Seconds())
}

// ObserveRedisError records a Redis error for a given algorithm.
func ObserveRedisError(algorithm string) {
	RedisErrorsTotal.WithLabelValues(algorithm).Inc()
}
