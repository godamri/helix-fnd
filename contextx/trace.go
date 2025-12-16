package contextx

import (
	"context"
)

type contextKey string

const (
	TraceIDKey contextKey = "helix_trace_id"
)

func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return "untriaged"
	}
	if id, ok := ctx.Value(TraceIDKey).(string); ok {
		return id
	}
	return "untriaged"
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}
