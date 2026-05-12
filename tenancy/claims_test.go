package tenancy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/tenancy"
)

func TestClaimsIsEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, (&tenancy.Claims{}).IsEmpty())
	require.True(t, (&tenancy.Claims{Skip: true}).IsEmpty(),
		"Skip alone does not carry tenancy")
	require.False(t, (&tenancy.Claims{TenantID: "t1"}).IsEmpty())
	require.False(t, (&tenancy.Claims{PartitionIDs: []string{"p1"}}).IsEmpty())
}

func TestExtendPartitionsDedupAndOrder(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1", "p2"}, AccessID: "a1", Skip: false}
	extended := base.ExtendPartitions("p3", "p2", "p4")

	require.NotSame(t, base, extended, "ExtendPartitions must return a new instance")
	require.Equal(t, []string{"p1", "p2"}, base.PartitionIDs, "base must be unchanged")
	require.Equal(t, []string{"p1", "p2", "p3", "p4"}, extended.PartitionIDs)
	require.Equal(t, "t1", extended.TenantID)
	require.Equal(t, "a1", extended.AccessID)
	require.False(t, extended.Skip)
}

func TestExtendPartitionsPreservesSkip(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", Skip: true}
	extended := base.ExtendPartitions("p1")
	require.True(t, extended.Skip, "Skip must be preserved across extension")
}

func TestExtendPartitionsNoOpWhenAllPresent(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1", "p2"}}
	extended := base.ExtendPartitions("p1", "p2")
	require.Equal(t, base.PartitionIDs, extended.PartitionIDs)
}

func TestExtendPartitionsIgnoresEmpty(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1"}}
	extended := base.ExtendPartitions("", "p2", "")
	require.Equal(t, []string{"p1", "p2"}, extended.PartitionIDs)
}

func TestIsEmptyOnNilReceiver(t *testing.T) {
	t.Parallel()
	var c *tenancy.Claims
	require.True(t, c.IsEmpty())
}

func TestExtendPartitionsOnNilReceiver(t *testing.T) {
	t.Parallel()
	var c *tenancy.Claims
	extended := c.ExtendPartitions("p1", "", "p2", "p1")
	require.NotNil(t, extended)
	require.Equal(t, []string{"p1", "p2"}, extended.PartitionIDs)
	require.Empty(t, extended.TenantID)
	require.False(t, extended.Skip)
}
