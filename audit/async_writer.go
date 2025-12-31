package audit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var ErrAuditBufferFull = errors.New("audit: buffer full, log dropped")

type AsyncLogger struct {
	events      chan Event
	writer      io.Writer
	wg          sync.WaitGroup
	logger      *slog.Logger
	closeOnce   sync.Once
	blockOnFull bool

	// Drop Strategy Stats
	dropCount   uint64
	lastLogTime atomic.Value
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
		blockOnFull: blockOnFull,
	}
	l.lastLogTime.Store(time.Unix(0, 0))

	l.wg.Add(1)
	go l.worker()

	return l
}

func (l *AsyncLogger) Log(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	if l.blockOnFull {
		// STRATEGY: High Consistency
		select {
		case l.events <- event:
			return nil
		case <-ctx.Done():
			l.handleDrop(event.Action)
			return ctx.Err()
		}
	} else {
		// STRATEGY: High Availability
		select {
		case l.events <- event:
			return nil
		default:
			l.handleDrop(event.Action)
			return ErrAuditBufferFull
		}
	}
}

func (l *AsyncLogger) handleDrop(action string) {
	atomic.AddUint64(&l.dropCount, 1)

	now := time.Now()
	lastLog, ok := l.lastLogTime.Load().(time.Time)
	if !ok {
		lastLog = time.Unix(0, 0)
	}

	// Rate-limited warning to stderr/slog to notify ops that the system is under pressure
	if now.Sub(lastLog) > 1*time.Minute {
		l.lastLogTime.Store(now)
		totalDropped := atomic.SwapUint64(&l.dropCount, 0)

		l.logger.Error("AUDIT_LOG_CRITICAL_FAILURE",
			slog.Uint64("dropped_count", totalDropped),
			slog.String("reason", "buffer_full_or_timeout"),
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
