package tenancy_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/tenancy"
	"github.com/pitabwire/frame/tests"
)

// okResponse returns a non-nil connect response used by tests so the
// noop handler doesn't violate the `nilnil` lint rule.
func okResponse() connect.AnyResponse {
	return connect.NewResponse(&wrapperspb.StringValue{Value: "ok"})
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
	wrapped := interceptor.WrapUnary(func(innerCtx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		got := tenancy.ClaimsFromContext(innerCtx)
		require.NotNil(t, got, "interceptor must bind claims into innerCtx")
		require.Equal(t, "T1", got.TenantID)
		require.Equal(t, []string{"P1"}, got.PartitionIDs)
		require.Equal(t, "A1", got.AccessID)
		require.False(t, got.Skip, "non-internal-system auth must yield Skip=false")
		return okResponse(), nil
	})

	_, err := wrapped(ctxAuth, nil)
	require.NoError(t, err)
}

// TestClaimsInterceptorWithNoAuthIsNoOp verifies the interceptor
// gracefully handles a request with no auth claims attached — it must
// not panic, and downstream tenancy.ClaimsFromContext must return nil.
func (s *InterceptorTestSuite) TestClaimsInterceptorWithNoAuthIsNoOp() {
	t := s.T()
	interceptor := tenancy.NewClaimsInterceptor()
	wrapped := interceptor.WrapUnary(func(innerCtx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		require.Nil(t, tenancy.ClaimsFromContext(innerCtx))
		return okResponse(), nil
	})
	_, err := wrapped(context.Background(), nil)
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
	wrapped := interceptor.WrapUnary(func(innerCtx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		got := tenancy.ClaimsFromContext(innerCtx)
		require.NotNil(t, got)
		require.True(t, got.Skip)
		return okResponse(), nil
	})

	_, err := wrapped(ctxAuth, nil)
	require.NoError(t, err)
}
