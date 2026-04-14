package telemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/telemetry"
)

func TestTenantAttributes_NoClaims(t *testing.T) {
	ctx := context.Background()
	attrs := telemetry.TenantAttributes(ctx)
	require.Nil(t, attrs)
}

func TestTenantAttributes_EmptyClaims(t *testing.T) {
	claims := &security.AuthenticationClaims{}
	ctx := claims.ClaimsToContext(context.Background())

	attrs := telemetry.TenantAttributes(ctx)
	require.Empty(t, attrs)
}

func TestTenantAttributes_TenantOnly(t *testing.T) {
	claims := &security.AuthenticationClaims{
		TenantID: "tenant-123",
	}
	ctx := claims.ClaimsToContext(context.Background())

	attrs := telemetry.TenantAttributes(ctx)
	require.Len(t, attrs, 1)
	require.Equal(t, attribute.Key("tenant_id"), attrs[0].Key)
	require.Equal(t, "tenant-123", attrs[0].Value.AsString())
}

func TestTenantAttributes_PartitionOnly(t *testing.T) {
	claims := &security.AuthenticationClaims{
		PartitionID: "partition-456",
	}
	ctx := claims.ClaimsToContext(context.Background())

	attrs := telemetry.TenantAttributes(ctx)
	require.Len(t, attrs, 1)
	require.Equal(t, attribute.Key("partition_id"), attrs[0].Key)
	require.Equal(t, "partition-456", attrs[0].Value.AsString())
}

func TestTenantAttributes_BothPresent(t *testing.T) {
	claims := &security.AuthenticationClaims{
		TenantID:    "tenant-123",
		PartitionID: "partition-456",
	}
	ctx := claims.ClaimsToContext(context.Background())

	attrs := telemetry.TenantAttributes(ctx)
	require.Len(t, attrs, 2)

	attrMap := make(map[attribute.Key]string)
	for _, a := range attrs {
		attrMap[a.Key] = a.Value.AsString()
	}

	require.Equal(t, "tenant-123", attrMap[telemetry.AttrTenantKey])
	require.Equal(t, "partition-456", attrMap[telemetry.AttrPartitionKey])
}

func TestTenantAttributes_FromExtMap(t *testing.T) {
	claims := &security.AuthenticationClaims{
		Ext: map[string]any{
			"tenant_id":    "ext-tenant",
			"partition_id": "ext-partition",
		},
	}
	ctx := claims.ClaimsToContext(context.Background())

	attrs := telemetry.TenantAttributes(ctx)
	require.Len(t, attrs, 2)

	attrMap := make(map[attribute.Key]string)
	for _, a := range attrs {
		attrMap[a.Key] = a.Value.AsString()
	}

	require.Equal(t, "ext-tenant", attrMap[telemetry.AttrTenantKey])
	require.Equal(t, "ext-partition", attrMap[telemetry.AttrPartitionKey])
}

func TestWithTenantAttributes_NoClaims(t *testing.T) {
	ctx := context.Background()
	opt := telemetry.WithTenantAttributes(ctx)
	require.NotNil(t, opt)
}

func TestWithTenantAttributes_WithClaims(t *testing.T) {
	claims := &security.AuthenticationClaims{
		TenantID:    "tenant-789",
		PartitionID: "partition-012",
	}
	ctx := claims.ClaimsToContext(context.Background())

	opt := telemetry.WithTenantAttributes(ctx)
	require.NotNil(t, opt)
}
