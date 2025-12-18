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

// AuditMiddleware records business events for non-GET requests.
// It captures Actor, Action, Resource, Status, and Payload (capped).
func AuditMiddleware(logger audit.Logger, cfg audit.Config) func(http.Handler) http.Handler {
	// Pre-process exclude paths for O(1) lookup map if list is long,
	// but slice iteration is fine for short lists.
	skipPaths := make(map[string]bool)
	for _, p := range cfg.ExcludePaths {
		skipPaths[strings.TrimSpace(p)] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip GET/OPTIONS (Read-only usually not audited in high-throughput, optional)
			if r.Method == http.MethodGet || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Skip Excluded Paths
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Body Capture with Configurable Limit
			var reqBody []byte
			if r.Body != nil && cfg.MaxBodySize > 0 {
				// Use the configured limit
				limitReader := io.LimitReader(r.Body, cfg.MaxBodySize)
				reqBody, _ = io.ReadAll(limitReader)

				// Restore body so the handler can read it.
				// We reconstruct the body using the bytes we read + the rest of the stream.
				r.Body = io.NopCloser(io.MultiReader(bytes.NewBuffer(reqBody), r.Body))
			}

			ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Async Log
			actorID := "anonymous"
			if u, ok := r.Context().Value(AuthPrincipalIDKey).(string); ok {
				actorID = u
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
					"status":     http.StatusText(ww.Status()),
					"ip":         r.RemoteAddr,
					"user_agent": r.UserAgent(),
				},
				NewValue: string(reqBody),
			}

			// Use background context to prevent cancellation
			_ = logger.Log(context.Background(), event)
		})
	}
}
