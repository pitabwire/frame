package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/pitabwire/frame/security"
)

// Attribute keys for multi-tenant context.
//
//nolint:gochecknoglobals // OpenTelemetry attribute keys must be global for reuse
var (
	AttrTenantKey    = attribute.Key("tenant_id")
	AttrPartitionKey = attribute.Key("partition_id")
)

// TenantAttributes extracts tenant_id and partition_id from the context's
// security claims and returns them as OpenTelemetry attributes.
// When claims are absent or fields are empty, those attributes are omitted
// rather than recorded as blank strings. This makes the function safe to call
// unconditionally — unauthenticated or pre-auth code paths simply get an
// empty slice.
func TenantAttributes(ctx context.Context) []attribute.KeyValue {
	claims := security.ClaimsFromContext(ctx)
	if claims == nil {
		return nil
	}

	var attrs []attribute.KeyValue

	if tid := claims.GetTenantID(); tid != "" {
		attrs = append(attrs, AttrTenantKey.String(tid))
	}

	if pid := claims.GetPartitionID(); pid != "" {
		attrs = append(attrs, AttrPartitionKey.String(pid))
	}

	return attrs
}

// WithTenantAttributes returns a metric.MeasurementOption that attaches
// tenant_id and partition_id from the context. Services use this when
// recording custom product metrics so every data point is automatically
// attributed to the correct tenant and partition.
//
// Usage:
//
//	counter.Add(ctx, 1,
//	    telemetry.WithTenantAttributes(ctx),
//	    metric.WithAttributes(attribute.String("login_method", "email")),
//	)
func WithTenantAttributes(ctx context.Context) metric.MeasurementOption {
	attrs := TenantAttributes(ctx)
	if len(attrs) == 0 {
		return metric.WithAttributes()
	}

	return metric.WithAttributes(attrs...)
}
