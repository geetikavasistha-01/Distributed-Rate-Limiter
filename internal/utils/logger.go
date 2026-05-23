package utils

import (
	"log/slog"
	"os"
)

// InitLogger initializes the global structured logger based on the environment.
// For production, we use JSON output. For development, we use Text/Structured console output.
func InitLogger(env string) *slog.Logger {
	var handler slog.Handler

	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
