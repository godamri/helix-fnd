package telemetry

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OTelHandler wraps a slog.Handler to inject OTel TraceID and AUTOMATICALLY record errors.
type OTelHandler struct {
	slog.Handler
}

func NewOTelHandler(h slog.Handler) *OTelHandler {
	return &OTelHandler{Handler: h}
}

// Handle intercepts every log to inject context and capture errors.
func (h *OTelHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)

	// Inject Trace Context (Correlation)
	if span.IsRecording() {
		sc := span.SpanContext()
		if sc.HasTraceID() {
			r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
		}
		if sc.HasSpanID() {
			r.AddAttrs(slog.String("span_id", sc.SpanID().String()))
		}

		// Auto-detect WARN and ERROR to enrich the Span
		if r.Level >= slog.LevelWarn {
			h.enrichSpan(span, r)
		}
	}

	return h.Handler.Handle(ctx, r)
}

// enrichSpan extracts error details from the log record and stamps them onto the OTel Span.
func (h *OTelHandler) enrichSpan(span trace.Span, r slog.Record) {
	// Attributes for OTel
	otelAttrs := make([]attribute.KeyValue, 0, r.NumAttrs())

	var errFound error

	r.Attrs(func(a slog.Attr) bool {
		// Map slog kinds to OTel types natively to avoid string allocation overhead
		switch a.Value.Kind() {
		case slog.KindString:
			otelAttrs = append(otelAttrs, attribute.String(a.Key, a.Value.String()))
		case slog.KindInt64:
			otelAttrs = append(otelAttrs, attribute.Int64(a.Key, a.Value.Int64()))
		case slog.KindFloat64:
			otelAttrs = append(otelAttrs, attribute.Float64(a.Key, a.Value.Float64()))
		case slog.KindBool:
			otelAttrs = append(otelAttrs, attribute.Bool(a.Key, a.Value.Bool()))
		default:
			// Fallback for objects/groups/etc
			otelAttrs = append(otelAttrs, attribute.String(a.Key, a.Value.String()))
		}

		// Check if this attribute is the error object
		if a.Key == "error" && a.Value.Kind() == slog.KindAny {
			if e, ok := a.Value.Any().(error); ok {
				errFound = e
			}
		}
		return true
	})

	// If no error object found in attributes but level is ERROR, use the message as the error
	if errFound == nil && r.Level >= slog.LevelError {
		errFound = errors.New(r.Message)
	}

	// Action based on Level
	if r.Level >= slog.LevelError {
		// Critical: Mark Span as Error and Record Exception
		span.RecordError(errFound, trace.WithAttributes(otelAttrs...))
		span.SetStatus(codes.Error, r.Message)
	} else if r.Level == slog.LevelWarn {
		// Warning: Just add an Event to the Trace (Yellow flag)
		span.AddEvent("log_warning", trace.WithAttributes(
			append(otelAttrs, attribute.String("message", r.Message))...,
		))
	}
}

// WithAttrs & WithGroup boilerplate to satisfy interface
func (h *OTelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &OTelHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *OTelHandler) WithGroup(name string) slog.Handler {
	return &OTelHandler{Handler: h.Handler.WithGroup(name)}
}
