package config

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// FileWatcher polls a file for changes.
// Why poll? Because fsnotify is often flaky on K8s mounted volumes due to symlink swapping strategies.
// Polling every 5s is cheap and reliable for config.
type FileWatcher struct {
	path     string
	interval time.Duration
	lastMod  time.Time
	logger   *slog.Logger
}

func NewFileWatcher(path string, interval time.Duration, logger *slog.Logger) *FileWatcher {
	return &FileWatcher{
		path:     path,
		interval: interval,
		logger:   logger,
	}
}

func (w *FileWatcher) Watch(ctx context.Context, onChange func()) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial stat
	if info, err := os.Stat(w.path); err == nil {
		w.lastMod = info.ModTime()
	}

	w.logger.Info("Config watcher started", "path", w.path)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				continue // File might be temporarily gone during swap
			}

			if info.ModTime().After(w.lastMod) {
				w.logger.Info("Config file changed, reloading...", "path", w.path)
				w.lastMod = info.ModTime()
				onChange()
			}
		}
	}
}
