package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/limiter"
	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/metrics"
)

// RateLimit creates an HTTP middleware that applies the provided rate limiter based on the client IP address.
func RateLimit(lim limiter.Limiter, cfg *limiter.LimiterConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			clientIP := extractIP(r)

			allowed, remaining, retryAfter, err := lim.Check(r.Context(), clientIP)
			duration := time.Since(start)

			if err != nil {
				metrics.ObserveRedisError(cfg.Algorithm)
				slog.Error("rate limit evaluation failed", "error", err, "ip", clientIP)
				// Fail open strategy: if Redis is down, we allow the request.
				next.ServeHTTP(w, r)
				return
			}

			status := "rejected"
			if allowed {
				status = "allowed"
			}
			metrics.ObserveRequest(cfg.Algorithm, status, duration)

			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(cfg.Limit, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(fmt.Sprintf(`{"error":"Too Many Requests","retry_after":"%s"}`, retryAfter.String())))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP determines the real IP address of the client, handling proxies.
func extractIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}
	
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
