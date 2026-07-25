package security_test

import (
	"testing"

	"github.com/pitabwire/frame/v2/security"
	"github.com/stretchr/testify/require"
)

func TestGetTenantPartitionIDsTrimWhitespace(t *testing.T) {
	t.Parallel()

	auth := &security.AuthenticationClaims{
		TenantID:    "  t1  ",
		PartitionID: " p1 ",
		AccessID:    " a1 ",
		Ext: map[string]any{
			"partition_ids": []string{" p2 ", "", "p1"},
		},
	}
	require.Equal(t, "t1", auth.GetTenantID())
	require.Equal(t, "p1", auth.GetPartitionID())
	require.Equal(t, "a1", auth.GetAccessID())
	require.Equal(t, []string{"p1", "p2"}, auth.GetPartitionIDs())
}

func TestGetPartitionIDsFromCommaSeparatedExt(t *testing.T) {
	t.Parallel()

	auth := &security.AuthenticationClaims{
		PartitionID: "p1",
		Ext:         map[string]any{"partition_ids": "p2, p3 ,p2"},
	}
	require.Equal(t, []string{"p1", "p2", "p3"}, auth.GetPartitionIDs())
}

func TestAsMetadataClaimsFromMapRoundTrip(t *testing.T) {
	t.Parallel()

	original := &security.AuthenticationClaims{
		TenantID:    "t1",
		PartitionID: "p1",
		AccessID:    "a1",
		ContactID:   "c1",
		DeviceID:    "d1",
		Roles:       []string{"admin", "member"},
		Ext: map[string]any{
			"partition_ids": []string{"p2", "p3"},
		},
	}
	original.Subject = "profile-1"
	original.ProfileID = "profile-1"

	meta := original.AsMetadata()
	require.Equal(t, "t1", meta["tenant_id"])
	require.Equal(t, "p1", meta["partition_id"])
	require.Equal(t, "p2,p3", meta["partition_ids"])
	require.Equal(t, "a1", meta["access_id"])
	require.Equal(t, "profile-1", meta["sub"])

	restored := security.ClaimsFromMap(meta)
	require.NotNil(t, restored)
	require.Equal(t, "t1", restored.GetTenantID())
	require.Equal(t, "p1", restored.GetPartitionID())
	require.Equal(t, []string{"p1", "p2", "p3"}, restored.GetPartitionIDs())
	require.Equal(t, "a1", restored.GetAccessID())
	require.Equal(t, "profile-1", restored.GetProfileID())
}

func TestAsMetadataSinglePartitionOmitsPartitionIDsKey(t *testing.T) {
	t.Parallel()

	auth := &security.AuthenticationClaims{TenantID: "t1", PartitionID: "p1"}
	meta := auth.AsMetadata()
	_, hasExtras := meta["partition_ids"]
	require.False(t, hasExtras, "single-partition claims should not emit partition_ids")
}

func TestClaimsFromMapTrimsWhitespace(t *testing.T) {
	t.Parallel()

	got := security.ClaimsFromMap(map[string]string{
		"sub":           "  profile-1  ",
		"tenant_id":     "  t1  ",
		"partition_id":  " p1 ",
		"partition_ids": " p2 , p3 ",
		"access_id":     " a1 ",
	})
	require.NotNil(t, got)
	require.Equal(t, "t1", got.GetTenantID())
	require.Equal(t, "p1", got.GetPartitionID())
	require.Equal(t, []string{"p1", "p2", "p3"}, got.GetPartitionIDs())
	require.Equal(t, "a1", got.GetAccessID())
	require.Equal(t, "profile-1", got.GetProfileID())
}

func TestClaimsFromMapNilWhenNoTenancyKeys(t *testing.T) {
	t.Parallel()
	require.Nil(t, security.ClaimsFromMap(map[string]string{"foo": "bar"}))
}
