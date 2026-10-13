package tenancy_test

import (
	"context"
	"testing"

	"connectrpc.com/connect/v2"
	"github.com/pitabwire/frame/v3/security"
	"github.com/pitabwire/frame/v3/tenancy"
	"github.com/pitabwire/frame/v3/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// invoke runs the claims interceptor around next as a unary RPC would.
func invoke(ctx context.Context, interceptor connect.ServerInterceptor, next func(context.Context) error) error {
	wrapped := interceptor(func(innerCtx context.Context, _ connect.Spec, _ connect.ServerStream) error {
		return next(innerCtx)
	})
	return wrapped(ctx, connect.Spec{StreamType: connect.StreamTypeUnary}, nil)
}

type InterceptorTestSuite struct {
	tests.BaseTestSuite
}

func TestInterceptorSuite(t *testing.T) {
	suite.Run(t, &InterceptorTestSuite{})
}

// TestClaimsInterceptorBindsClaims verifies the end-to-end binding
// chain: an authentication interceptor attaches *security.AuthenticationClaims
// to ctx; the claims interceptor then derives *tenancy.Claims and binds
// it to the child ctx. Downstream code reads tenancy.ClaimsFromContext
// and sees the expected tenancy.
//
// RLS-filtering behaviour is already exercised by Task 14's provider
// integration tests against real Postgres — this test focuses on the
// interceptor chain itself, which is pure context plumbing.
func (s *InterceptorTestSuite) TestClaimsInterceptorBindsClaims() {
	t := s.T()
	auth := &security.AuthenticationClaims{
		TenantID:    "T1",
		PartitionID: "P1",
		AccessID:    "A1",
	}
	ctxAuth := auth.ClaimsToContext(context.Background())

	interceptor := tenancy.NewClaimsInterceptor()
	err := invoke(ctxAuth, interceptor, func(innerCtx context.Context) error {
		got := tenancy.ClaimsFromContext(innerCtx)
		require.NotNil(t, got, "interceptor must bind claims into innerCtx")
		require.Equal(t, "T1", got.TenantID)
		require.Equal(t, []string{"P1"}, got.PartitionIDs)
		require.Equal(t, "A1", got.AccessID)
		require.False(t, got.Skip, "non-internal-system auth must yield Skip=false")
		return nil
	})
	require.NoError(t, err)
}

// TestClaimsInterceptorWithNoAuthIsNoOp verifies the interceptor
// gracefully handles a request with no auth claims attached — it must
// not panic, and downstream tenancy.ClaimsFromContext must return nil.
func (s *InterceptorTestSuite) TestClaimsInterceptorWithNoAuthIsNoOp() {
	t := s.T()
	interceptor := tenancy.NewClaimsInterceptor()
	err := invoke(context.Background(), interceptor, func(innerCtx context.Context) error {
		require.Nil(t, tenancy.ClaimsFromContext(innerCtx))
		return nil
	})
	require.NoError(t, err)
}

// TestClaimsInterceptorSkipForInternalSystem verifies internal-system
// callers yield Skip=true through the full chain.
func (s *InterceptorTestSuite) TestClaimsInterceptorSkipForInternalSystem() {
	t := s.T()
	auth := &security.AuthenticationClaims{
		TenantID:    "T1",
		PartitionID: "P1",
		Roles:       []string{security.ConstantSystemInternalRole},
	}
	ctxAuth := auth.ClaimsToContext(context.Background())

	interceptor := tenancy.NewClaimsInterceptor()
	err := invoke(ctxAuth, interceptor, func(innerCtx context.Context) error {
		got := tenancy.ClaimsFromContext(innerCtx)
		require.NotNil(t, got)
		require.True(t, got.Skip)
		return nil
	})
	require.NoError(t, err)
}

// TestClaimsInterceptorRequiredClaimsFails verifies a secure binder rejects a
// call with no bindable tenancy as FailedPrecondition and keeps the cause.
func (s *InterceptorTestSuite) TestClaimsInterceptorRequiredClaimsFails() {
	t := s.T()
	interceptor := tenancy.NewClaimsInterceptorWithBinder(tenancy.NewClaimsBinder(true))
	called := false
	err := invoke(context.Background(), interceptor, func(context.Context) error {
		called = true
		return nil
	})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	require.ErrorIs(t, err, tenancy.ErrClaimsRequired)
}
