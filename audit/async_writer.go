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
	events    chan Event
	writer    io.Writer
	wg        sync.WaitGroup
	logger    *slog.Logger
	closeOnce sync.Once

	// Config
	blockOnFull bool

	// Drop Strategy Metrics
	dropCount   uint64
	lastLogTime time.Time
	dropMu      sync.Mutex
}

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
		blockOnFull: blockOnFull, // Injected Config
		lastLogTime: time.Now(),
	}

	l.wg.Add(1)
	go l.worker()

	return l
}

func (l *AsyncLogger) Log(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	if l.blockOnFull {
		// MODE: GUARANTEED DELIVERY
		// Will block if buffer is full. Use with caution on high-throughput.
		select {
		case l.events <- event:
			return nil
		case <-ctx.Done():
			// Even in blocking mode, respect context cancellation (timeout)
			l.handleDrop(event.Action + "_ctx_cancelled")
			return ctx.Err()
		}
	} else {
		// STANDARD MODE: BEST EFFORT
		select {
		case l.events <- event:
			return nil
		default:
			l.handleDrop(event.Action)
			return nil
		}
	}
}

func (l *AsyncLogger) handleDrop(action string) {
	currentDrops := atomic.AddUint64(&l.dropCount, 1)

	if time.Since(l.lastLogTime) < 5*time.Second {
		return
	}

	l.dropMu.Lock()
	defer l.dropMu.Unlock()

	if time.Since(l.lastLogTime) >= 5*time.Second {
		l.logger.Warn("CRITICAL: Audit log buffer full/dropped.",
			"strategy", "drop_on_full",
			"total_dropped", currentDrops,
			"sample_action", action,
		)
		atomic.StoreUint64(&l.dropCount, 0)
		l.lastLogTime = time.Now()
	}
}

func (l *AsyncLogger) worker() {
	defer l.wg.Done()
	encoder := json.NewEncoder(l.writer)

	for event := range l.events {
		// Retry logic for file writes could go here, but for stdout we just write.
		if err := encoder.Encode(event); err != nil {
			l.logger.Error("Failed to write audit log to IO", "error", err)
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
