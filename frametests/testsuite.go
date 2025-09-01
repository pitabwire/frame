package frametests

import (
	"context"
	"testing"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"go.uber.org/mock/gomock"
)

// FrameBaseTestSuite provides a base test suite with all necessary test components.
type FrameBaseTestSuite struct {
	suite.Suite
	Network   *testcontainers.DockerNetwork
	resources []definition.TestResource
	Ctrl      *gomock.Controller

	InitResourceFunc func(ctx context.Context) []definition.TestResource
}

// SetupSuite initialises the test environment for the test suite.
func (s *FrameBaseTestSuite) SetupSuite() {
	t := s.T()

	s.Ctrl = gomock.NewController(t)

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
	options []*definition.DependancyOption,
	testFn func(t *testing.T, db *definition.DependancyOption)) {
	for _, opt := range options {
		t.Run(opt.Name(), func(tt *testing.T) {
			testFn(tt, opt)
		})
	}
}
