package audit

import (
	"context"
	"time"
)

// Event represents a business audit event.
type Event struct {
	ActorID   string            `json:"actor_id"` // Who? (User UUID, System)
	Action    string            `json:"action"`   // Did What? (CREATE_ORDER, DELETE_USER)
	Resource  string            `json:"resource"` // On What? (Order:123)
	OldValue  interface{}       `json:"old_value,omitempty"`
	NewValue  interface{}       `json:"new_value,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
}

// Logger defines where the audit log goes (Console, File, Kafka).
type Logger interface {
	Log(ctx context.Context, event Event) error
}

// NoopLogger is for dev/testing.
type NoopLogger struct{}

func (n *NoopLogger) Log(ctx context.Context, event Event) error {
	return nil
}
