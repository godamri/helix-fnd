package middleware

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/godamri/helix-fnd/pkg/contextx"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	TraceHeader   = "X-Trace-Id"
	RequestHeader = "X-Request-Id"
)

func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		span := trace.SpanFromContext(ctx)
		var traceID string

		if span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
		}
		if traceID == "" {
			traceID = r.Header.Get(TraceHeader)
		}
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

		ctx = contextx.WithTraceID(ctx, traceID)
		ctx = contextx.WithRequestID(ctx, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GRPCTraceInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID := span.SpanContext().TraceID().String()
		ctx = contextx.WithTraceID(ctx, traceID)
	} else {
		uid := uuid.New()
		ctx = contextx.WithTraceID(ctx, hex.EncodeToString(uid[:]))
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if v := md.Get(strings.ToLower(RequestHeader)); len(v) > 0 {
			ctx = contextx.WithRequestID(ctx, v[0])
		} else {
			ctx = contextx.WithRequestID(ctx, uuid.NewString())
		}
	} else {
		ctx = contextx.WithRequestID(ctx, uuid.NewString())
	}

	return handler(ctx, req)
}
