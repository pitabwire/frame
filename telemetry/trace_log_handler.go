package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceContextHandler is an slog.Handler that injects trace_id and span_id
// from the OTel span context into every log record. This enables correlation
// between stdout/stderr logs and distributed traces in production.
type TraceContextHandler struct {
	inner slog.Handler
}

// NewTraceContextHandler wraps an existing slog.Handler to inject trace context.
func NewTraceContextHandler(inner slog.Handler) slog.Handler {
	return &TraceContextHandler{inner: inner}
}

// TraceContextHandlerWrapper returns a function suitable for util.WithLogHandlerWrapper
// that wraps any slog.Handler with trace context injection.
func TraceContextHandlerWrapper() func(slog.Handler) slog.Handler {
	return NewTraceContextHandler
}

func (h *TraceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *TraceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	if sc.HasSpanID() {
		r.AddAttrs(slog.String("span_id", sc.SpanID().String()))
	}
	return h.inner.Handle(ctx, r)
}

func (h *TraceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *TraceContextHandler) WithGroup(name string) slog.Handler {
	return &TraceContextHandler{inner: h.inner.WithGroup(name)}
}
