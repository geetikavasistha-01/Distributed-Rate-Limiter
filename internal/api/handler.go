package api

import (
	"encoding/json"
	"net/http"

	"github.com/geetikavasistha-01/Distributed-Rate-Limiter/internal/redis"
)

// HealthResponse represents the JSON payload structure of the health endpoint.
type HealthResponse struct {
	Status  string            `json:"status"`
	Service string            `json:"service"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks"`
}

// HealthHandler returns an HTTP handler function for the health endpoint.
func HealthHandler(version string, rdb redis.RedisClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		redisStatus := "healthy"
		httpStatus := http.StatusOK
		status := "ok"

		// Query Redis liveness using the context propagated from middlewares
		if err := rdb.Ping(r.Context()); err != nil {
			redisStatus = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			status = "error"
		}

		w.WriteHeader(httpStatus)

		resp := HealthResponse{
			Status:  status,
			Service: "distributed-rate-limiter",
			Version: version,
			Checks: map[string]string{
				"redis": redisStatus,
			},
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}
