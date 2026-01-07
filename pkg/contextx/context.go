package contextx

import (
	"context"
)

type contextKey string

const (
	AuthPrincipalIDKey contextKey = "helix.auth_principal_id" // sub (siapa)
	AuthSessionIDKey   contextKey = "helix.auth_session_id"   // jti / sid (tiket sesi mana)
	AuthDecisionIDKey  contextKey = "helix.auth_decision_id"  // reference ke keputusan AuthZ (audit trail)

	TraceIDKey       contextKey = "helix.trace_id"
	ParentTraceIDKey contextKey = "helix.parent_trace_id"
	SpanIDKey        contextKey = "helix.span_id"
	RequestIDKey     contextKey = "helix.request_id"
	SourceServiceKey contextKey = "helix.source_service"
	EntryPointKey    contextKey = "helix.entry_point" // http | grpc | cron | consumer

	IdempotencyKey  contextKey = "helix.idempotency_key"
	RetryAttemptKey contextKey = "helix.retry_attempt"
	AuditReasonKey  contextKey = "helix.audit_reason"
	ChangeTicketKey contextKey = "helix.change_ticket"
)

func GetTraceID(ctx context.Context) string { return getString(ctx, TraceIDKey, "untriaged") }
func WithTraceID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, TraceIDKey, v)
}

func GetParentTraceID(ctx context.Context) string { return getString(ctx, ParentTraceIDKey, "") }
func WithParentTraceID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, ParentTraceIDKey, v)
}

func GetSpanID(ctx context.Context) string { return getString(ctx, SpanIDKey, "") }
func WithSpanID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, SpanIDKey, v)
}

func GetRequestID(ctx context.Context) string { return getString(ctx, RequestIDKey, "") }
func WithRequestID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, RequestIDKey, v)
}

func GetSourceService(ctx context.Context) string { return getString(ctx, SourceServiceKey, "unknown") }
func WithSourceService(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, SourceServiceKey, v)
}

func GetEntryPoint(ctx context.Context) string { return getString(ctx, EntryPointKey, "unknown") }
func WithEntryPoint(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, EntryPointKey, v)
}

func GetAuthPrincipalID(ctx context.Context) string { return getString(ctx, AuthPrincipalIDKey, "") }
func WithAuthPrincipalID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, AuthPrincipalIDKey, v)
}

func GetAuthSessionID(ctx context.Context) string { return getString(ctx, AuthSessionIDKey, "") }
func WithAuthSessionID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, AuthSessionIDKey, v)
}

func GetAuthDecisionID(ctx context.Context) string { return getString(ctx, AuthDecisionIDKey, "") }
func WithAuthDecisionID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, AuthDecisionIDKey, v)
}

func GetIdempotencyKey(ctx context.Context) string { return getString(ctx, IdempotencyKey, "") }
func WithIdempotencyKey(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, IdempotencyKey, v)
}

func GetRetryAttempt(ctx context.Context) int { return getInt(ctx, RetryAttemptKey, 0) }
func WithRetryAttempt(ctx context.Context, v int) context.Context {
	return context.WithValue(ctx, RetryAttemptKey, v)
}

func GetAuditReason(ctx context.Context) string { return getString(ctx, AuditReasonKey, "") }
func WithAuditReason(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, AuditReasonKey, v)
}

func GetChangeTicket(ctx context.Context) string { return getString(ctx, ChangeTicketKey, "") }
func WithChangeTicket(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, ChangeTicketKey, v)
}

func getString(ctx context.Context, key contextKey, fallback string) string {
	if ctx == nil {
		return fallback
	}
	if val, ok := ctx.Value(key).(string); ok {
		return val
	}
	return fallback
}

func getInt(ctx context.Context, key contextKey, fallback int) int {
	if ctx == nil {
		return fallback
	}
	if val, ok := ctx.Value(key).(int); ok {
		return val
	}
	return fallback
}

func getStringSlice(ctx context.Context, key contextKey) []string {
	if ctx == nil {
		return nil
	}
	if val, ok := ctx.Value(key).([]string); ok {
		return val
	}
	return nil
}
