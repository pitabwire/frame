package frametests

import (
	"context"
	"testing"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/pitabwire/frame/frametests/definition"
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
