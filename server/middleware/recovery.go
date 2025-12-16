package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// PanicRecovery handles panics in HTTP handlers.
// It logs the stack trace with context and returns a 500 error.
// CRITICAL: This does NOT call os.Exit(1). The server must stay alive.
func PanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// 1. Capture Stack Trace
				stack := string(debug.Stack())

				// 2. Log with Context (TraceID via slog.ErrorContext)
				// We use Error level because a panic is an application bug.
				slog.ErrorContext(r.Context(), "HTTP PANIC RECOVERED",
					"error", fmt.Sprintf("%v", rec),
					"method", r.Method,
					"path", r.URL.Path,
					"stack", stack,
				)

				// 3. Return 500 to client
				// Do not leak the stack trace to the client (Security).
				http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
