package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/gcerrors"
)

// Common attribute keys used across the frame.
//
//nolint:gochecknoglobals // OpenTelemetry attribute keys must be global for reuse
var (
	methodKey  = attribute.Key("app_method")
	packageKey = attribute.Key("app_package")
	statusKey  = attribute.Key("app_status")
	errorKey   = attribute.Key("app_error")
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	startTimeContextKey  contextKey = "spanStartTimeCtxKey"
	methodNameContextKey contextKey = "methodNameCtxKey"
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
// The caller is responsible for ending the span.
//
//nolint:spancheck // OpenTelemetry spans are intentionally returned to caller for proper lifecycle management
func (t *Tracer) Start(ctx context.Context, methodName string) (context.Context, trace.Span) {
	fullName := t.pkg + "." + methodName

	sCtx, span := t.tracer.Start(ctx, fullName, trace.WithAttributes(methodKey.String(methodName)))
	sCtx = context.WithValue(sCtx, startTimeContextKey, time.Now())
	return context.WithValue(sCtx, methodNameContextKey, fullName), span
}

// End completes a span with error information if applicable.
func (t *Tracer) End(ctx context.Context, span trace.Span, err error) {
	startTimeValue := ctx.Value(startTimeContextKey)
	startTime, ok := startTimeValue.(time.Time)
	if !ok {
		util.Log(ctx).Error(
			"invalid startTime context value",
			"value", startTimeValue,
		)
		return
	}
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

	methodNameValue := ctx.Value(methodNameContextKey)
	methodName, ok := methodNameValue.(string)
	if !ok {
		util.Log(ctx).Error(
			"invalid methodName context value",
			"value", methodNameValue,
		)
		return
	}

	t.latencyMeasure.Record(ctx,
		float64(elapsed.Milliseconds()),

		metric.WithAttributes(
			statusKey.String(fmt.Sprint(code)),
			methodKey.String(methodName)),
	)
}
