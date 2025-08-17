package framedata

import (
	"context"
	"testing"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/stretchr/testify/suite"
)

// HealthTestSuite extends FrameBaseTestSuite for health testing with real dependencies
type HealthTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestHealthCheckerCreation tests health checker creation with real dependencies
func (s *HealthTestSuite) TestHealthCheckerCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "health_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "health-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("HealthCheckerCreation", func(t *testing.T) {
			// Test health checker creation
			checker := NewHealthChecker(nil, nil)
			s.NotNil(checker, "Should create health checker")
		})

		t.Run("HealthCheckerInterfaceCompliance", func(t *testing.T) {
			// Test that our health checker implements the interface correctly
			var _ HealthChecker = NewHealthChecker(nil, nil)
		})
	})
}

// TestHealthCheckerOperations tests health checker operations with real dependencies
func (s *HealthTestSuite) TestHealthCheckerOperations() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "health_operations_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "health-operations-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("HealthCheckWithNilDB", func(t *testing.T) {
			// Test health check with nil database
			checker := NewHealthChecker(nil, nil)
			status := checker.CheckHealth(ctx)
			s.False(status.IsHealthy, "Should not be healthy with nil database")
		})

		t.Run("HealthCheckerMonitoring", func(t *testing.T) {
			// Test health checker monitoring
			checker := NewHealthChecker(nil, nil)
			s.False(checker.IsMonitoring(), "Should not be monitoring initially")
		})
	})
}

func TestHealthTestSuite(t *testing.T) {
	suite.Run(t, new(HealthTestSuite))
}
