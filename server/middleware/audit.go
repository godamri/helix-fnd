package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/godamri/helix-fnd/audit"
	"github.com/godamri/helix-fnd/pkg/contextx"
)

func AuditMiddleware(logger audit.Logger, cfg audit.Config) func(http.Handler) http.Handler {
	skipPaths := make(map[string]bool)
	for _, p := range cfg.ExcludePaths {
		skipPaths[strings.TrimSpace(p)] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			var reqBody []byte
			if r.Body != nil && cfg.MaxBodySize > 0 {
				limitReader := io.LimitReader(r.Body, cfg.MaxBodySize)
				reqBody, _ = io.ReadAll(limitReader)
				r.Body = io.NopCloser(io.MultiReader(bytes.NewBuffer(reqBody), r.Body))
			}

			ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			actorID := contextx.GetActorID(r.Context())
			if actorID == "" {
				actorID = "anonymous"
			}

			path := r.URL.Path
			if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
				path = rctx.RoutePattern()
			}

			event := audit.Event{
				ActorID:   actorID,
				Action:    r.Method,
				Resource:  path,
				Timestamp: start,
				TraceID:   contextx.GetTraceID(r.Context()),
				Metadata: map[string]string{
					"status":          http.StatusText(ww.Status()),
					"ip":              r.RemoteAddr,
					"user_agent":      r.UserAgent(),
					"actor_type":      contextx.GetActorType(r.Context()),
					"org_id":          contextx.GetOrgID(r.Context()),
					"entry_point":     contextx.GetEntryPoint(r.Context()),
					"request_id":      contextx.GetRequestID(r.Context()),
					"auth_method":     contextx.GetAuthMethod(r.Context()),
					"idempotency_key": contextx.GetIdempotencyKey(r.Context()),
				},
				NewValue: string(reqBody),
			}

			_ = logger.Log(context.Background(), event)
		})
	}
}
