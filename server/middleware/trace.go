package middleware

import (
	"encoding/hex"
	"net/http"

	"github.com/godamri/helix-fnd/pkg/contextx"
	"github.com/google/uuid"
)

const (
	TraceHeader   = "X-Trace-Id"
	RequestHeader = "X-Request-Id"
)

func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get(TraceHeader)
		if traceID == "" {
			uid := uuid.New()
			traceID = hex.EncodeToString(uid[:])
		}

		reqID := r.Header.Get(RequestHeader)
		if reqID == "" {
			reqID = uuid.NewString()
		}

		w.Header().Set(TraceHeader, traceID)
		w.Header().Set(RequestHeader, reqID)

		ctx := r.Context()
		ctx = contextx.WithTraceID(ctx, traceID)
		ctx = contextx.WithRequestID(ctx, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
