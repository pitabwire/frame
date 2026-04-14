package telemetry

import (
	"context"
	"errors"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Common attribute keys used across the frame.
//
//nolint:gochecknoglobals // OpenTelemetry attribute keys must be global for reuse
var (
	AttrMethodKey  = attribute.Key("frame_method")
	AttrPackageKey = attribute.Key("frame_package")
	AttrStatusKey  = attribute.Key("frame_status")
	AttrErrorKey   = attribute.Key("frame_error")
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	startTimeContextKey  contextKey = "spanStartTimeCtxKey"
	methodNameContextKey contextKey = "methodNameCtxKey"
)

// tracer provides OpenTelemetry tracing for services.
type tracer struct {
	name           string
	tracer         trace.Tracer
	latencyMeasure metric.Float64Histogram
}

// NewTracer creates a new tracer for a package.
func NewTracer(name string, options ...trace.TracerOption) Tracer {
	otelTracer := otel.Tracer(name, options...)

	return &tracer{
		name:           name,
		tracer:         otelTracer,
		latencyMeasure: LatencyMeasure(name),
	}
}

// Start creates and starts a new span and returns the updated context and span.
// The caller is responsible for ending the span.
//
//nolint:spancheck // OpenTelemetry spans are intentionally returned to caller for proper lifecycle management
func (t *tracer) Start(
	ctx context.Context,
	spanName string,
	options ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	fullName := t.name + "/" + spanName

	options = append(options, trace.WithAttributes(AttrMethodKey.String(spanName)))

	sCtx, span := t.tracer.Start(ctx, spanName, options...)
	sCtx = context.WithValue(sCtx, startTimeContextKey, time.Now())
	return context.WithValue(sCtx, methodNameContextKey, fullName), span
}

// End completes a span with error information if applicable.
// When security claims are present in the context, tenant_id and partition_id
// are automatically added to both the span attributes and the latency metric.
func (t *tracer) End(ctx context.Context, span trace.Span, err error, options ...trace.SpanEndOption) {
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

	// Extract tenant attributes from context claims (nil-safe).
	tenantAttrs := TenantAttributes(ctx)

	if err != nil {
		options = append(options, trace.WithStackTrace(true))

		span.SetAttributes(
			AttrErrorKey.String(err.Error()),
		)

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	if len(tenantAttrs) > 0 {
		span.SetAttributes(tenantAttrs...)
	}

	span.End(options...)

	methodNameValue := ctx.Value(methodNameContextKey)
	methodName, ok := methodNameValue.(string)
	if !ok {
		util.Log(ctx).Error(
			"invalid methodName context value",
			"value", methodNameValue,
		)
		return
	}

	metricAttrs := append([]attribute.KeyValue{
		AttrStatusKey.String(ErrorCode(err)),
		AttrMethodKey.String(methodName),
	}, tenantAttrs...)

	t.latencyMeasure.Record(ctx,
		float64(elapsed.Milliseconds()),
		metric.WithAttributes(metricAttrs...),
	)
}

func ErrorCode(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline exceeded"
	}
	return "err"
}
