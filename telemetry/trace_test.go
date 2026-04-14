package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/telemetry"
)

func setupTracerWithRecorder(t *testing.T) (telemetry.Tracer, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	// Register the test provider globally so NewTracer picks it up.
	otelTP := tp
	originalTP := setGlobalTracerProvider(otelTP)
	t.Cleanup(func() { setGlobalTracerProvider(originalTP) })

	tr := telemetry.NewTracer("test/pkg")
	return tr, recorder
}

func TestEnd_WithTenantClaims(t *testing.T) {
	tr, recorder := setupTracerWithRecorder(t)

	claims := &security.AuthenticationClaims{
		TenantID:    "tenant-abc",
		PartitionID: "partition-xyz",
	}
	ctx := claims.ClaimsToContext(context.Background())

	ctx, span := tr.Start(ctx, "TestOp")
	tr.End(ctx, span, nil)

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	attrMap := spanAttrMap(spans[0].Attributes())
	require.Equal(t, "tenant-abc", attrMap[telemetry.AttrTenantKey])
	require.Equal(t, "partition-xyz", attrMap[telemetry.AttrPartitionKey])
}

func TestEnd_WithoutClaims(t *testing.T) {
	tr, recorder := setupTracerWithRecorder(t)

	ctx := context.Background()
	ctx, span := tr.Start(ctx, "TestOp")
	tr.End(ctx, span, nil)

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	attrMap := spanAttrMap(spans[0].Attributes())
	_, hasTenant := attrMap[telemetry.AttrTenantKey]
	_, hasPartition := attrMap[telemetry.AttrPartitionKey]
	require.False(t, hasTenant)
	require.False(t, hasPartition)
}

func TestEnd_WithError(t *testing.T) {
	tr, recorder := setupTracerWithRecorder(t)

	claims := &security.AuthenticationClaims{
		TenantID:    "tenant-err",
		PartitionID: "partition-err",
	}
	ctx := claims.ClaimsToContext(context.Background())

	ctx, span := tr.Start(ctx, "FailingOp")
	tr.End(ctx, span, errors.New("something failed"))

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	attrMap := spanAttrMap(spans[0].Attributes())
	require.Equal(t, "tenant-err", attrMap[telemetry.AttrTenantKey])
	require.Equal(t, "partition-err", attrMap[telemetry.AttrPartitionKey])
	require.Equal(t, "something failed", attrMap[telemetry.AttrErrorKey])
}

func TestEnd_PartialClaims(t *testing.T) {
	tr, recorder := setupTracerWithRecorder(t)

	claims := &security.AuthenticationClaims{
		TenantID: "tenant-only",
	}
	ctx := claims.ClaimsToContext(context.Background())

	ctx, span := tr.Start(ctx, "PartialOp")
	tr.End(ctx, span, nil)

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	attrMap := spanAttrMap(spans[0].Attributes())
	require.Equal(t, "tenant-only", attrMap[telemetry.AttrTenantKey])
	_, hasPartition := attrMap[telemetry.AttrPartitionKey]
	require.False(t, hasPartition)
}

func spanAttrMap(attrs []attribute.KeyValue) map[attribute.Key]string {
	m := make(map[attribute.Key]string, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Value.AsString()
	}
	return m
}
