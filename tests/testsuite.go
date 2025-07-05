package tests

import (
	"context"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"go.uber.org/mock/gomock"
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
	// Using log.Logger instead of fmt.Print to avoid linting issues
	lc.logger.Printf("%s", string(l.Content))
}

// FrameBaseTestSuite provides a base test suite with all necessary test components.
type FrameBaseTestSuite struct {
	suite.Suite

	Deps     []Dependancy
	MockCtrl *gomock.Controller
}

// SetupSuite initialises the test environment for the test suite.
func (s *FrameBaseTestSuite) SetupSuite() {
	t := s.T()

	s.MockCtrl = gomock.NewController(t)

	ctx := t.Context()

	for _, dep := range s.Deps {
		err := dep.Setup(ctx)
		require.NoError(t, err, "could not setup tests")
	}
}

// TearDownSuite cleans up resources after all tests are completed.
func (s *FrameBaseTestSuite) TearDownSuite() {
	if s.MockCtrl != nil {
		s.MockCtrl.Finish()
	}

	t := s.T()
	ctx := t.Context()

	for _, dep := range s.Deps {
		dep.Cleanup(ctx)
	}
}
