package utils

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	loggerInstance *slog.Logger
	loggerOnce     sync.Once
)

func Logger() *slog.Logger {
	loggerOnce.Do(func() {
		cfg := LoadConfig()
		loggerInstance = slog.New(newHandler(cfg))
	})

	return loggerInstance
}

func newHandler(cfg Config) slog.Handler {
	isDevelopment := strings.EqualFold(cfg.NodeEnv, "development")

	level := slog.LevelInfo
	if isDevelopment {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}

	if isDevelopment {
		return slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.NewJSONHandler(os.Stdout, opts)
}
