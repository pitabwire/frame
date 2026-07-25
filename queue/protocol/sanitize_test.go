package protocol_test

import (
	"testing"

	"github.com/pitabwire/frame/v2/queue/protocol"
	"github.com/stretchr/testify/require"
)

func TestSanitizeInboundStripsClaimsWhenUntrusted(t *testing.T) {
	t.Parallel()
	md := map[string]string{
		"roles":        "admin",
		"tenant_id":    "evil",
		"sub":          "forged",
		"traceparent":  "00-abc",
		"ce-type":      "com.example.t",
		"content-type": "application/json",
	}
	out := protocol.SanitizeInbound(md, false)
	require.NotContains(t, out, "roles")
	require.NotContains(t, out, "tenant_id")
	require.NotContains(t, out, "sub")
	require.Equal(t, "00-abc", out["traceparent"])
	require.Equal(t, "com.example.t", out["ce-type"])
}

func TestSanitizeInboundKeepsClaimsWhenTrusted(t *testing.T) {
	t.Parallel()
	md := map[string]string{"roles": "admin", "tenant_id": "t1"}
	out := protocol.SanitizeInbound(md, true)
	require.Equal(t, "admin", out["roles"])
	require.Equal(t, "t1", out["tenant_id"])
}

func TestIsClaimKeyIncludesPartitionIDs(t *testing.T) {
	t.Parallel()
	require.True(t, protocol.IsClaimKey("partition_ids"))
	require.True(t, protocol.IsClaimKey("partition_id"))
}

func TestApplyClaimTrustTenancyOnlyStripsRoles(t *testing.T) {
	t.Parallel()
	md := map[string]string{
		"tenant_id":     "t1",
		"partition_id":  "p1",
		"partition_ids": "p2",
		"roles":         "internal",
		"service_name":  "svc",
		"traceparent":   "00-x",
	}
	out := protocol.ApplyClaimTrust(md, protocol.TrustTenancyOnly)
	require.Equal(t, "t1", out["tenant_id"])
	require.Equal(t, "p1", out["partition_id"])
	require.Equal(t, "p2", out["partition_ids"])
	require.NotContains(t, out, "roles")
	require.NotContains(t, out, "service_name")
	require.Equal(t, "00-x", out["traceparent"])
}

func TestSanitizeInboundStripsPartitionIDsWhenUntrusted(t *testing.T) {
	t.Parallel()
	md := map[string]string{"partition_ids": "p2,p3", "tenant_id": "t1"}
	out := protocol.SanitizeInbound(md, false)
	require.NotContains(t, out, "partition_ids")
	require.NotContains(t, out, "tenant_id")
}
