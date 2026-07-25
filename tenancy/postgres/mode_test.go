package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeConn records session exec calls for unit tests without Postgres.
type fakeConn struct {
	execs []string
	err   error
}

func (f *fakeConn) Exec(_ context.Context, sql string, _ ...any) error {
	f.execs = append(f.execs, sql)
	return f.err
}

type hookCapture struct {
	acquire dialect.AcquireHook
}

func (h *hookCapture) Name() string { return "stub" }
func (h *hookCapture) NormalizeDSN(raw string) (string, error) {
	return raw, nil
}
func (h *hookCapture) OpenConnection(
	context.Context, string, dialect.ConnectionOptions,
) (gorm.Dialector, *sql.DB, func() error, error) {
	err := errors.New("stub: OpenConnection not used")
	// Return a non-nil close so callers can always invoke it; never nil+err.
	closeFn := func() error { return err }
	return nil, nil, closeFn, err
}
func (h *hookCapture) QuoteIdentifier(s string) string { return s }
func (h *hookCapture) RegisterAcquireHook(hook dialect.AcquireHook) error {
	h.acquire = hook
	return nil
}
func (h *hookCapture) RegisterReleaseHook(dialect.ReleaseHook) error { return nil }
func (h *hookCapture) AdvisoryLock(context.Context, *gorm.DB, int64) (func(), error) {
	return func() {}, nil
}
func (h *hookCapture) IsRelationAlreadyExistsErr(error) bool { return false }

func TestBeforeAcquireSecureModes(t *testing.T) {
	t.Parallel()

	p := tenpg.New(
		tenpg.WithSecurityMode(tenancy.ModeHybrid),
		tenpg.WithAllowGlobalServices("admin-svc"),
	)
	require.Equal(t, tenancy.ModeHybrid, p.SecurityMode())

	ad := &hookCapture{}
	require.NoError(t, p.WireAdapter(ad))
	require.NotNil(t, ad.acquire)
	acq := ad.acquire

	// Missing claims → ErrClaimsRequired
	err := acq(context.Background(), &fakeConn{})
	require.ErrorIs(t, err, tenancy.ErrClaimsRequired)

	// Bindable claims → bind
	ctx := tenancy.WithClaims(context.Background(), &tenancy.Claims{
		TenantID: "t1", PartitionIDs: []string{"p1"},
	})
	conn := &fakeConn{}
	require.NoError(t, acq(ctx, conn))
	require.NotEmpty(t, conn.execs)

	// Bare Skip → ErrSkipNotPermitted
	ctx = tenancy.WithSkipEnforcement(context.Background())
	err = acq(ctx, &fakeConn{})
	require.ErrorIs(t, err, tenancy.ErrSkipNotPermitted)

	// Partition-only → ErrTenantIDRequired
	ctx = tenancy.WithClaims(context.Background(), &tenancy.Claims{PartitionIDs: []string{"p1"}})
	err = acq(ctx, &fakeConn{})
	require.ErrorIs(t, err, tenancy.ErrTenantIDRequired)

	// Tenant-wide (empty partitions) allowed
	ctx = tenancy.WithClaims(context.Background(), &tenancy.Claims{TenantID: "t1"})
	require.NoError(t, acq(ctx, &fakeConn{}))

	// Scoped SystemPrincipal without user claims
	ctx = tenancy.WithSystemPrincipal(context.Background(), tenancy.SystemPrincipal{
		ServiceName: "job", TenantID: "t9", PartitionIDs: []string{"p9"},
	})
	conn = &fakeConn{}
	require.NoError(t, acq(ctx, conn))
	require.NotEmpty(t, conn.execs)

	// AllowGlobal without allowlist → denied
	ctx = tenancy.WithSystemPrincipal(context.Background(), tenancy.SystemPrincipal{
		ServiceName: "evil", AllowGlobal: true, Reason: "migration",
	})
	err = acq(ctx, &fakeConn{})
	require.ErrorIs(t, err, tenancy.ErrAllowGlobalDenied)

	// AllowGlobal with allowlisted service
	ctx = tenancy.WithSystemPrincipal(context.Background(), tenancy.SystemPrincipal{
		ServiceName: "admin-svc", AllowGlobal: true, Reason: "export",
	})
	require.NoError(t, acq(ctx, &fakeConn{}))

	// Framework migration marker authorizes AllowGlobal
	ctx = tenancy.WithFrameworkMigration(context.Background())
	require.NoError(t, acq(ctx, &fakeConn{}))

	// FailOpen still allows missing claims
	pOpen := tenpg.New(tenpg.WithSecurityMode(tenancy.ModeFailOpen))
	ad2 := &hookCapture{}
	require.NoError(t, pOpen.WireAdapter(ad2))
	require.NoError(t, ad2.acquire(context.Background(), &fakeConn{}))
}
