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
