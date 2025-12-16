package audit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// AsyncLogger implements Logger interface with non-blocking writes.
type AsyncLogger struct {
	events    chan Event
	writer    io.Writer
	wg        sync.WaitGroup
	logger    *slog.Logger
	closeOnce sync.Once
}

// NewAsyncLogger creates a logger that writes to w asynchronously.
// bufferSize determines how many events can be queued before blocking (backpressure).
func NewAsyncLogger(w io.Writer, bufferSize int, logger *slog.Logger) *AsyncLogger {
	if w == nil {
		w = os.Stdout
	}
	l := &AsyncLogger{
		events: make(chan Event, bufferSize),
		writer: w,
		logger: logger,
	}

	l.wg.Add(1)
	go l.worker()

	return l
}

func (l *AsyncLogger) Log(ctx context.Context, event Event) error {
	// Set default timestamp if empty
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Non-blocking send (drop if full? or block? For audit, block is safer, or separate metric drop)
	// Here we choose to block to guarantee audit trail (Backpressure applied to API)
	select {
	case l.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *AsyncLogger) worker() {
	defer l.wg.Done()
	encoder := json.NewEncoder(l.writer)

	for event := range l.events {
		if err := encoder.Encode(event); err != nil {
			l.logger.Error("Failed to write audit log", "error", err)
		}
	}
}

// Close flushes the channel and waits for worker to finish.
func (l *AsyncLogger) Close() error {
	l.closeOnce.Do(func() {
		close(l.events)
	})
	l.wg.Wait()
	return nil
}
