package config

import (
	"log/slog"
	"os"
	"strings"
)

// InitLogger initializes the global structured logger based on the config.
func InitLogger(env, logLevel string) *slog.Logger {
	var handler slog.Handler

	level := slog.LevelInfo
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
