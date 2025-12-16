package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Runner encapsulates the startup logic.
// It handles signals and context cancellation so you don't have to write it 50 times.
type Runner struct {
	Logger *slog.Logger
}

func NewRunner(logger *slog.Logger) *Runner {
	return &Runner{Logger: logger}
}

// Run executes the main logic function. It provides a context that cancels on SIGTERM/SIGINT.
func (r *Runner) Run(fn func(ctx context.Context) error) {
	// Create context that listens for the kill signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r.Logger.Info("Service starting...")

	if err := fn(ctx); err != nil {
		r.Logger.Error("Service startup failed", "error", err)
		stop()
		os.Exit(1)
	}

	<-ctx.Done()

	// Graceful shutdown period
	r.Logger.Info("Shutdown signal received. Cleaning up...")
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r.Logger.Info("Service shutdown complete.")
}
