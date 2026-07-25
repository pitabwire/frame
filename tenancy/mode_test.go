package tenancy_test

import (
	"context"
	"testing"

	"github.com/pitabwire/frame/v2/security"
	"github.com/pitabwire/frame/v2/tenancy"
	"github.com/stretchr/testify/require"
)

func TestParseSecurityMode(t *testing.T) {
	t.Parallel()
	m, ok := tenancy.ParseSecurityMode("hybrid")
	require.True(t, ok)
	require.Equal(t, tenancy.ModeHybrid, m)
	require.True(t, m.IsSecure())

	m, ok = tenancy.ParseSecurityMode("fail_closed")
	require.True(t, ok)
	require.Equal(t, tenancy.ModeFailClosed, m)

	m, ok = tenancy.ParseSecurityMode("")
	require.False(t, ok)
	require.Equal(t, tenancy.ModeFailOpen, m)
}

func TestIsBindable(t *testing.T) {
	t.Parallel()
	require.False(t, (*tenancy.Claims)(nil).IsBindable())
	require.False(t, (&tenancy.Claims{Skip: true, TenantID: "t1"}).IsBindable())
	require.False(t, (&tenancy.Claims{PartitionIDs: []string{"p1"}}).IsBindable())
	require.True(t, (&tenancy.Claims{TenantID: "t1"}).IsBindable())
	require.True(t, (&tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1"}}).IsBindable())
}

func TestClaimsFromAuthHonorInternalSkipOption(t *testing.T) {
	t.Parallel()
	auth := &security.AuthenticationClaims{
		TenantID: "t1", PartitionID: "p1",
		Roles: []string{security.ConstantSystemInternalRole},
	}
	// Legacy ClaimsToContext sets SkipTenancyChecks for internal → Skip.
	legacyCtx := auth.ClaimsToContext(context.Background())
	legacy := tenancy.ClaimsFromAuth(legacyCtx, auth)
	require.True(t, legacy.Skip)

	// WithoutInternalTenancySkip + ClaimsFromContext must not Skip (binds RLS).
	secureCtx := auth.ClaimsToContext(context.Background(), security.WithoutInternalTenancySkip())
	viaFallback := tenancy.ClaimsFromContext(secureCtx)
	require.NotNil(t, viaFallback)
	require.False(t, viaFallback.Skip, "internal JWT without skip flag must bind RLS")
	require.Equal(t, "t1", viaFallback.TenantID)

	secure := tenancy.ClaimsFromAuth(legacyCtx, auth, tenancy.WithHonorInternalSkip(false))
	require.False(t, secure.Skip)
}

func TestSystemPrincipalAndFrameworkMigration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	require.False(t, tenancy.IsFrameworkMigration(ctx))

	ctx = tenancy.WithFrameworkMigration(ctx)
	require.True(t, tenancy.IsFrameworkMigration(ctx))
	sp, ok := tenancy.SystemPrincipalFromContext(ctx)
	require.True(t, ok)
	require.True(t, sp.AllowGlobal)
	require.Equal(t, "frame", sp.ServiceName)

	// App-forged Reason alone is not a framework marker.
	ctx2 := tenancy.WithSystemPrincipal(context.Background(), tenancy.SystemPrincipal{
		Reason: "migration", AllowGlobal: true, ServiceName: "evil",
	})
	require.False(t, tenancy.IsFrameworkMigration(ctx2))
}

func TestClaimsBinderRequireClaims(t *testing.T) {
	t.Parallel()
	b := tenancy.NewClaimsBinder(true)
	_, err := b.Bind(context.Background())
	require.ErrorIs(t, err, tenancy.ErrClaimsRequired)

	auth := &security.AuthenticationClaims{TenantID: "t1", PartitionID: "p1"}
	ctx := auth.ClaimsToContext(context.Background(), security.WithoutInternalTenancySkip())
	ctx, err = b.Bind(ctx)
	require.NoError(t, err)
	got := tenancy.ClaimsFromContext(ctx)
	require.True(t, got.IsBindable())
}

func TestEnsureFrameworkMigration(t *testing.T) {
	t.Parallel()
	parent := tenancy.WithFrameworkMigration(context.Background())
	job := context.Background()
	merged := tenancy.EnsureFrameworkMigration(job, parent)
	require.True(t, tenancy.IsFrameworkMigration(merged))
}
