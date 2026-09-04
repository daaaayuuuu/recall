package logging

import (
	"log/slog"
	"os"
	"strings"
)

func New(environment, configuredLevel string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(configuredLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With("environment", environment)
}
