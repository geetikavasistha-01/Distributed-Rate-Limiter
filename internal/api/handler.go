package api

import (
	"encoding/json"
	"net/http"
)

// HealthResponse represents the JSON payload structure of the health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

// HealthHandler returns an HTTP handler function for the health endpoint.
func HealthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		resp := HealthResponse{
			Status:  "ok",
			Service: "distributed-rate-limiter",
			Version: version,
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}
