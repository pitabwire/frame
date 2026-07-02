package frame_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/datastore/pool"
	"github.com/pitabwire/frame/v2/frametests"
	"github.com/pitabwire/frame/v2/frametests/definition"
	"github.com/pitabwire/frame/v2/tenancy"
	"github.com/pitabwire/frame/v2/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// fakeProvider is a no-op tenancy.Provider used to confirm that
// WithTenancyProvider's override actually reaches pool.NewPool. It
// counts WireAdapter invocations so the test can assert the wiring.
type fakeProvider struct {
	wireAdapterCalls atomic.Int32
}

func (*fakeProvider) Name() string                       { return "fake" }
func (*fakeProvider) Capabilities() tenancy.Capabilities { return tenancy.Capabilities{} }
func (*fakeProvider) Install(_ context.Context, _ *gorm.DB, _ []tenancy.ModelInfo) error {
	return nil
}
func (p *fakeProvider) WireAdapter(_ dialect.DialectAdapter) error {
	p.wireAdapterCalls.Add(1)
	return nil
}
func (*fakeProvider) WireGorm(_ *gorm.DB) error { return nil }

// DatastoreOptionsTestSuite covers wiring guarantees for the
// frame-level datastore option helpers — particularly that
// WithTenancyProvider's chosen provider reaches the underlying pool.
type DatastoreOptionsTestSuite struct {
	tests.BaseTestSuite
}

func TestDatastoreOptionsSuite(t *testing.T) {
	suite.Run(t, &DatastoreOptionsTestSuite{})
}

// TestWithTenancyProviderOverrideReachesPool guards the regression
// described in the audit: WithTenancyProvider must actually flow
// through to pool.NewPool so that WireAdapter is called on the
// supplied provider (not just the default Postgres-RLS provider).
func (s *DatastoreOptionsTestSuite) TestWithTenancyProviderOverrideReachesPool() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		myProv := &fakeProvider{}
		ctx, svc := frame.NewService(
			frame.WithName("override-test"),
			frametests.WithNoopDriver(),
			frame.WithTenancyProvider(myProv),
			frame.WithDatastore(pool.WithConnection(dsn, false)),
		)
		defer svc.Stop(ctx)

		require.Same(t, myProv, svc.TenancyProvider(),
			"svc.TenancyProvider should return the override")
		require.Equal(t, int32(1), myProv.wireAdapterCalls.Load(),
			"the provider's WireAdapter must be called exactly once")
	})
}

// TestWithTenancyProviderNilDisablesEnforcement verifies that passing
// nil to WithTenancyProvider truly disables tenancy hooks (i.e. no
// default Postgres-RLS provider sneaks in).
func (s *DatastoreOptionsTestSuite) TestWithTenancyProviderNilDisablesEnforcement() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		ctx, svc := frame.NewService(
			frame.WithName("disable-test"),
			frametests.WithNoopDriver(),
			frame.WithTenancyProvider(nil),
			frame.WithDatastore(pool.WithConnection(dsn, false)),
		)
		defer svc.Stop(ctx)

		require.Nil(t, svc.TenancyProvider(),
			"svc.TenancyProvider should be nil when explicitly disabled")
	})
}
