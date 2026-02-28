package frametests

import (
	"context"
	"fmt"
	"testing"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/security"
)

// FrameBaseTestSuite provides a base test suite with all necessary test components.
type FrameBaseTestSuite struct {
	suite.Suite
	Network   *testcontainers.DockerNetwork
	resources []definition.TestResource

	InitResourceFunc func(ctx context.Context) []definition.TestResource
}

// SetupSuite initialises the test environment for the test suite.
func (s *FrameBaseTestSuite) SetupSuite() {
	t := s.T()

	ctx := t.Context()

	log := util.Log(ctx)

	net, err := network.New(ctx)

	require.NoError(t, err, "could not create network")
	s.Network = net

	if s.InitResourceFunc == nil {
		require.NotNil(t, s.InitResourceFunc, "InitResourceFunc is required")
	}

	s.resources = s.InitResourceFunc(ctx)

	for _, dep := range s.resources {
		log.WithField("image", dep.Name()).Info("Setting up container...")
		err = dep.Setup(ctx, net)
		require.NoError(t, err, "could not setup tests")
	}
}

func (s *FrameBaseTestSuite) Resources() []definition.DependancyConn {
	var deps []definition.DependancyConn
	for _, dep := range s.resources {
		deps = append(deps, dep)
	}

	return deps
}

// TearDownSuite cleans up resources after all tests are completed.
func (s *FrameBaseTestSuite) TearDownSuite() {
	t := s.T()
	ctx := t.Context()

	for _, dep := range s.resources {
		dep.Cleanup(ctx)
	}

	if s.Network != nil {
		err := s.Network.Remove(ctx)
		require.NoError(t, err, "could not remove network")
	}
}

// WithTestDependencies runs subtests for each provided DependencyOption.
// It ensures each subtest is properly isolated and runs in parallel if desired.
func WithTestDependencies(t *testing.T,
	options []*definition.DependencyOption,
	testFn func(t *testing.T, opt *definition.DependencyOption)) {
	t.Helper() // Marks this as a helper so test failures point to caller
	for _, opt := range options {
		nueOpt := opt
		t.Run(nueOpt.Name(), func(tt *testing.T) {
			testFn(tt, nueOpt)
		})
	}
}

// WithAuthClaims creates a context with fully populated authentication claims
// for test scenarios that require tenant-scoped operations.
func (s *FrameBaseTestSuite) WithAuthClaims(ctx context.Context, tenantID, partitionID, profileID string) context.Context {
	claims := &security.AuthenticationClaims{
		TenantID:    tenantID,
		PartitionID: partitionID,
		AccessID:    util.IDString(),
		ContactID:   profileID,
		SessionID:   util.IDString(),
		DeviceID:    "test-device",
	}
	claims.Subject = profileID

	return claims.ClaimsToContext(ctx)
}

// SeedTenantAccess writes a tenancy_access member tuple so the profile can pass
// the TenancyAccessChecker (data access layer). The tenancyAccessNamespace is
// typically "tenancy_access".
func (s *FrameBaseTestSuite) SeedTenantAccess(
	ctx context.Context,
	auth security.Authorizer,
	tenancyAccessNamespace, tenantID, partitionID, profileID string,
) {
	tenancyPath := fmt.Sprintf("%s/%s", tenantID, partitionID)
	err := auth.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: tenancyAccessNamespace, ID: tenancyPath},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: profileID},
	})
	s.Require().NoError(err, "failed to seed tenant access")
}
