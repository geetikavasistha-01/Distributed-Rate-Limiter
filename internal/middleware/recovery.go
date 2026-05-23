package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery recovers from any panic in downstream handlers, logs the traceback with context,
// and returns an HTTP 500 Internal Server Error.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				reqID := GetRequestID(r.Context())
				stack := debug.Stack()

				slog.Error("panic recovered",
					slog.Any("error", err),
					slog.String("request_id", reqID),
					slog.String("stack", string(stack)),
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
				)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
