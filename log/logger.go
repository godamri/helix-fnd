package log

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

type Config struct {
	Level  string `envconfig:"LOG_LEVEL" default:"info"`
	Format string `envconfig:"LOG_FORMAT" default:"json"`
}

// sensitiveKeys defines fields that must be redacted.
var sensitiveKeys = map[string]bool{
	"password":      true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"authorization": true,
	"secret":        true,
	"db_dsn":        true,
	"api_key":       true,
}

// Redactor filters sensitive keys from log output.
func Redactor(groups []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	if sensitiveKeys[key] {
		return slog.Attr{
			Key:   a.Key,
			Value: slog.StringValue("[REDACTED]"),
		}
	}
	return a
}

func New(cfg Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
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
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:       level,
			TimeFormat:  time.TimeOnly,
			ReplaceAttr: Redactor,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       level,
			ReplaceAttr: Redactor,
		})
	}

	return slog.New(handler)
}
