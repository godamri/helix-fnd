package middleware

import (
	"net/http"

	"github.com/godamri/helix-fnd/contextx"
	"github.com/google/uuid"
)

const (
	TraceHeader = "X-Trace-Id"
)

func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get(TraceHeader)

		if traceID == "" {
			traceID = uuid.NewString()
		}

		w.Header().Set(TraceHeader, traceID)

		ctx := contextx.WithTraceID(r.Context(), traceID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
