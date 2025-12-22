package audit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type AsyncLogger struct {
	events      chan Event
	writer      io.Writer
	wg          sync.WaitGroup
	logger      *slog.Logger
	closeOnce   sync.Once
	blockOnFull bool

	// Drop Strategy Stats
	dropCount   uint64
	lastLogTime atomic.Value // Stores time.Time
}

// NewAsyncLogger creates a logger.
// If blockOnFull is true, logging will BLOCK if the buffer is full (High Consistency).
// If false, it drops logs to preserve availability (High Availability).
func NewAsyncLogger(w io.Writer, bufferSize int, blockOnFull bool, logger *slog.Logger) *AsyncLogger {
	if w == nil {
		w = os.Stdout
	}
	if bufferSize <= 0 {
		bufferSize = 1024
	}

	l := &AsyncLogger{
		events:      make(chan Event, bufferSize),
		writer:      w,
		logger:      logger,
		blockOnFull: blockOnFull,
	}
	l.lastLogTime.Store(time.Unix(0, 0))

	l.wg.Add(1)
	go l.worker()

	return l
}

// Log attempts to queue an event.
func (l *AsyncLogger) Log(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	if l.blockOnFull {
		// STRATEGY: High Consistency (Payment/Ledger)
		// We block until space is available or context is cancelled.
		select {
		case l.events <- event:
			return nil
		case <-ctx.Done():
			// Context cancelled (timeout/shutdown)
			l.handleDrop(event.Action)
			return ctx.Err()
		}
	} else {
		// STRATEGY: High Availability (General logging)
		// Try to send, if full -> drop immediately.
		select {
		case l.events <- event:
			return nil
		default:
			// Buffer full. Drop log and increment counter for metrics.
			l.handleDrop(event.Action)
			return nil
		}
	}
}

// handleDrop performs rate-limited logging (1 log per 5 minutes) to prevent log flooding.
func (l *AsyncLogger) handleDrop(action string) {
	atomic.AddUint64(&l.dropCount, 1)

	now := time.Now()
	lastLog, ok := l.lastLogTime.Load().(time.Time)
	if !ok {
		lastLog = time.Unix(0, 0)
	}

	if now.Sub(lastLog) > 5*time.Minute {
		l.lastLogTime.Store(now)
		totalDropped := atomic.SwapUint64(&l.dropCount, 0)

		l.logger.Warn("AUDIT_LOG_DROPPED",
			slog.Uint64("dropped_count", totalDropped),
			slog.String("reason", "buffer_full"),
			slog.String("sample_action", action),
			slog.Bool("blocking_mode", l.blockOnFull),
		)
	}
}

func (l *AsyncLogger) worker() {
	defer l.wg.Done()
	encoder := json.NewEncoder(l.writer)

	for event := range l.events {
		if err := encoder.Encode(event); err != nil {
			l.logger.Error("audit_write_failed", slog.String("err", err.Error()))
		}
	}
}

func (l *AsyncLogger) Close() error {
	l.closeOnce.Do(func() {
		close(l.events)
	})
	l.wg.Wait()
	return nil
}
