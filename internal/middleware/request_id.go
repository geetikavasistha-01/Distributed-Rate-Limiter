package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type contextKey string

// RequestIDKey is the context key for storing the Request ID.
const RequestIDKey contextKey = "request_id"

// GenerateRequestID creates a cryptographically secure random request ID.
func GenerateRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "fallback-req-id"
	}
	return hex.EncodeToString(bytes)
}

// RequestID attaches a unique X-Request-ID header to the response and propagates it in the request Context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = GenerateRequestID()
		}

		// Set the header in response
		w.Header().Set("X-Request-ID", reqID)

		// Inject into context
		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the Request ID from the context.
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}
