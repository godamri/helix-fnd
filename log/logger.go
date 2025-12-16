package log

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

type Config struct {
	Level  string `envconfig:"LOG_LEVEL" default:"info"`  // debug, info, warn, error
	Format string `envconfig:"LOG_FORMAT" default:"json"` // json, console
}

func New(cfg Config) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler

	if cfg.Format == "console" {
		// Pretty Print for Local Development
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:      level,
			TimeFormat: time.TimeOnly,
		})
	} else {
		// JSON for Production (Machine Readable)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	return slog.New(handler)
}
