package contextx

import (
	"context"
)

type contextKey string

const (
	AuthPrincipalIDKey contextKey = "helix.auth_principal_id"

	TraceIDKey       contextKey = "helix.trace_id"
	ParentTraceIDKey contextKey = "helix.parent_trace_id"
	SpanIDKey        contextKey = "helix.span_id"
	RequestIDKey     contextKey = "helix.request_id"
	SourceServiceKey contextKey = "helix.source_service"
	EntryPointKey    contextKey = "helix.entry_point" // http | grpc | cron | consumer

	OrgIDKey      contextKey = "helix.org_id"     // Tenant/Merchant ID
	UserIDKey     contextKey = "helix.user_id"    // End-user
	ActorIDKey    contextKey = "helix.actor_id"   // Who is calling (Service Name or User ID)
	ActorTypeKey  contextKey = "helix.actor_type" // human | service | cron | system
	SessionIDKey  contextKey = "helix.session_id" // Login session
	UserEmailKey  contextKey = "helix.user_email"
	AuthMethodKey contextKey = "helix.auth_method" // jwt | api_key | mtls

	RoleKey          contextKey = "helix.role"
	PermissionsKey   contextKey = "helix.permissions" // []string
	PolicyVersionKey contextKey = "helix.policy_ver"
	RegionKey        contextKey = "helix.region"
	JurisdictionKey  contextKey = "helix.jurisdiction"
	DataClassKey     contextKey = "helix.data_class" // public | internal | financial | pii

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

func GetOrgID(ctx context.Context) string { return getString(ctx, OrgIDKey, "") }
func WithOrgID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, OrgIDKey, v)
}

func GetUserID(ctx context.Context) string { return getString(ctx, UserIDKey, "") }
func WithUserID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, UserIDKey, v)
}

func GetActorID(ctx context.Context) string { return getString(ctx, ActorIDKey, "") }
func WithActorID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, ActorIDKey, v)
}

func GetActorType(ctx context.Context) string { return getString(ctx, ActorTypeKey, "unknown") }
func WithActorType(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, ActorTypeKey, v)
}

func GetSessionID(ctx context.Context) string { return getString(ctx, SessionIDKey, "") }
func WithSessionID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, SessionIDKey, v)
}

func GetUserEmail(ctx context.Context) string { return getString(ctx, UserEmailKey, "") }
func WithUserEmail(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, UserEmailKey, v)
}

func GetAuthMethod(ctx context.Context) string { return getString(ctx, AuthMethodKey, "") }
func WithAuthMethod(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, AuthMethodKey, v)
}

func GetRole(ctx context.Context) string { return getString(ctx, RoleKey, "") }
func WithRole(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, RoleKey, v)
}

func GetPermissions(ctx context.Context) []string { return getStringSlice(ctx, PermissionsKey) }
func WithPermissions(ctx context.Context, v []string) context.Context {
	return context.WithValue(ctx, PermissionsKey, v)
}

func GetPolicyVersion(ctx context.Context) string { return getString(ctx, PolicyVersionKey, "") }
func WithPolicyVersion(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, PolicyVersionKey, v)
}

func GetRegion(ctx context.Context) string { return getString(ctx, RegionKey, "") }
func WithRegion(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, RegionKey, v)
}

func GetJurisdiction(ctx context.Context) string { return getString(ctx, JurisdictionKey, "") }
func WithJurisdiction(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, JurisdictionKey, v)
}

func GetDataClass(ctx context.Context) string { return getString(ctx, DataClassKey, "internal") }
func WithDataClass(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, DataClassKey, v)
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

func WithIdentity(ctx context.Context, userID, orgID, email, actorType string, roles []string) context.Context {
	ctx = WithUserID(ctx, userID)
	ctx = WithOrgID(ctx, orgID)
	ctx = WithUserEmail(ctx, email)
	ctx = WithActorType(ctx, actorType)
	ctx = WithPermissions(ctx, roles)
	if userID != "" {
		ctx = WithActorID(ctx, userID)
	}
	return ctx
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
