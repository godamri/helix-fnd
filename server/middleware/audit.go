package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/godamri/helix-fnd/audit"
	"github.com/godamri/helix-fnd/pkg/contextx"
)

// AuditMiddleware records business events for non-GET requests.
// It captures Actor (from Context), Action (Method + Path), and Status.
func AuditMiddleware(logger audit.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// For Enterprise, we might want to limit this or skip sensitive paths.
			// Here is a simple implementation that reads and restores body.
			var reqBody []byte
			if r.Body != nil {
				reqBody, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}

			ww := NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Assumption: UserID is injected into context by Auth Middleware
			actorID := "anonymous"
			if u, ok := r.Context().Value("user_id").(string); ok {
				actorID = u
			}

			path := r.URL.Path
			if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
				path = rctx.RoutePattern() // Use pattern "/users/{id}" instead of raw path
			}

			event := audit.Event{
				ActorID:   actorID,
				Action:    r.Method,
				Resource:  path,
				Timestamp: start,
				TraceID:   contextx.GetTraceID(r.Context()),
				Metadata: map[string]string{
					"status":     http.StatusText(ww.Status()),
					"ip":         r.RemoteAddr,
					"user_agent": r.UserAgent(),
				},
				// Note: OldValue/NewValue usually requires Application Layer logic.
				// Middleware can only capture "Payload" (NewValue candidate).
				NewValue: string(reqBody),
			}

			// Use background context because request context is cancelled after handler returns
			_ = logger.Log(context.Background(), event)
		})
	}
}
