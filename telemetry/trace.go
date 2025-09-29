package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/gcerrors"
)

// Common attribute keys used across the frame.
var (
	methodKey  = attribute.Key("app_method")
	packageKey = attribute.Key("app_package")
	statusKey  = attribute.Key("app_status")
	errorKey   = attribute.Key("app_error")
)

const (
	startTimeContextKey  = "spanStartTimeCtxKey"
	methodNameContextKey = "methodNameCtxKey"
)

// Tracer provides OpenTelemetry tracing for services.
type Tracer struct {
	pkg            string
	tracer         trace.Tracer
	latencyMeasure metric.Float64Histogram
}

// NewTracer creates a new Tracer for a package.
func NewTracer(pkg string) *Tracer {

	attrs := []attribute.KeyValue{
		packageKey.String(pkg),
	}

	tracer := otel.Tracer(pkg, trace.WithInstrumentationAttributes(attrs...))

	return &Tracer{
		pkg:            pkg,
		tracer:         tracer,
		latencyMeasure: LatencyMeasure(pkg),
	}
}

// Start creates and starts a new span and returns the updated context and span.
func (t *Tracer) Start(ctx context.Context, methodName string) (context.Context, trace.Span) {
	fullName := t.pkg + "." + methodName

	sCtx, span := t.tracer.Start(ctx, fullName, trace.WithAttributes(methodKey.String(methodName)))
	sCtx = context.WithValue(sCtx, startTimeContextKey, time.Now())
	return context.WithValue(sCtx, methodNameContextKey, fullName), span
}

// End completes a span with error information if applicable.
func (t *Tracer) End(ctx context.Context, span trace.Span, err error) {
	startTime := ctx.Value(startTimeContextKey).(time.Time)
	elapsed := time.Since(startTime)

	code := gcerrors.OK

	if err != nil {
		code = gcerrors.Code(err)
		span.SetAttributes(
			errorKey.String(err.Error()),
			statusKey.String(fmt.Sprint(code)),
		)
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()

	methodName := ctx.Value(methodNameContextKey).(string)

	t.latencyMeasure.Record(ctx,
		float64(elapsed.Milliseconds()),

		metric.WithAttributes(
			statusKey.String(fmt.Sprint(code)),
			methodKey.String(methodName)),
	)
}
