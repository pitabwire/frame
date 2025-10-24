package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type Tracer interface {
	Start(ctx context.Context, methodName string, options ...trace.SpanStartOption) (context.Context, trace.Span)
	End(ctx context.Context, span trace.Span, err error, options ...trace.SpanEndOption)
}
