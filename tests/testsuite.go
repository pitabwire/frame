package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/mock/gomock"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests/testdef"
)

// StdoutLogConsumer is a LogConsumer that prints the log to stdout.
type StdoutLogConsumer struct {
	logger *util.LogEntry
}

// NewStdoutLogConsumer creates a new StdoutLogConsumer.
func NewStdoutLogConsumer(ctx context.Context) *StdoutLogConsumer {
	return &StdoutLogConsumer{
		logger: util.Log(ctx),
	}
}

// Accept prints the log to stdout.
func (lc *StdoutLogConsumer) Accept(l testcontainers.Log) {
	switch l.LogType {
	case "STDERR":
		lc.logger.Error(string(l.Content))
	default:
		lc.logger.Info(string(l.Content))
	}
	lc.logger.Printf("%s", string(l.Content))
}

// FrameBaseTestSuite provides a base test suite with all necessary test components.
type FrameBaseTestSuite struct {
	suite.Suite

	MigrationImageContext string
	Network               *testcontainers.DockerNetwork
	Resources             []testdef.TestResource
	Ctrl                  *gomock.Controller
}

// SetupSuite initialises the test environment for the test suite.
func (s *FrameBaseTestSuite) SetupSuite() {
	t := s.T()

	s.Ctrl = gomock.NewController(t)

	ctx := t.Context()

	net, err := network.New(ctx)
	require.NoError(t, err, "could not create network")
	s.Network = net

	for _, dep := range s.Resources {
		err = dep.Setup(ctx, net)
		require.NoError(t, err, "could not setup tests")
	}
}

func (s *FrameBaseTestSuite) Migrate(ctx context.Context, ds frame.DataSource) error {
	if s.MigrationImageContext == "" {
		s.MigrationImageContext = "../../"
	}

	cRequest := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context: s.MigrationImageContext,
		},
		ConfigModifier: func(config *container.Config) {
			config.Env = []string{
				"LOG_LEVEL=debug",
				"DO_MIGRATION=true",
				fmt.Sprintf("DATABASE_URL=%s", ds.String()),
			}
		},
		Networks:   []string{s.Network.ID},
		WaitingFor: wait.ForExit(),
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{NewStdoutLogConsumer(ctx)},
		},
	}

	migrationC, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: cRequest,
			Started:          true,
		})
	if err != nil {
		return err
	}

	return migrationC.Terminate(ctx)
}

// TearDownSuite cleans up resources after all tests are completed.
func (s *FrameBaseTestSuite) TearDownSuite() {
	if s.Ctrl != nil {
		s.Ctrl.Finish()
	}

	t := s.T()
	ctx := t.Context()

	for _, dep := range s.Resources {
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
