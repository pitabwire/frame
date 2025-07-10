package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"go.uber.org/mock/gomock"

	"github.com/pitabwire/frame/tests/testdef"
)

// FrameBaseTestSuite provides a base test suite with all necessary test components.
type FrameBaseTestSuite struct {
	suite.Suite
	Network   *testcontainers.DockerNetwork
	resources []testdef.TestResource
	Ctrl      *gomock.Controller

	InitResourceFunc func(ctx context.Context) []testdef.TestResource
}

// SetupSuite initialises the test environment for the test suite.
func (s *FrameBaseTestSuite) SetupSuite() {
	t := s.T()

	s.Ctrl = gomock.NewController(t)

	ctx := t.Context()

	net, err := network.New(ctx)
	require.NoError(t, err, "could not create network")
	s.Network = net

	if s.InitResourceFunc == nil {
		require.NotNil(t, s.InitResourceFunc, "InitResourceFunc is required")
	}

	s.resources = s.InitResourceFunc(ctx)

	for _, dep := range s.resources {
		err = dep.Setup(ctx, net)
		require.NoError(t, err, "could not setup tests")
	}
}

func (s *FrameBaseTestSuite) Resources() []testdef.DependancyConn {
	var deps []testdef.DependancyConn
	for _, dep := range s.resources {
		deps = append(deps, dep)
	}

	return deps
}

// TearDownSuite cleans up resources after all tests are completed.
func (s *FrameBaseTestSuite) TearDownSuite() {
	if s.Ctrl != nil {
		s.Ctrl.Finish()
	}

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

// WithTestDependancies Creates subtests with each known DependancyOption.
func WithTestDependancies(t *testing.T,
	options []*testdef.DependancyOption,
	testFn func(t *testing.T, db *testdef.DependancyOption)) {
	for _, opt := range options {
		t.Run(opt.Name(), func(tt *testing.T) {
			testFn(tt, opt)
		})
	}
}
