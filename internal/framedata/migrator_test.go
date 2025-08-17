package framedata

import (
	"context"
	"testing"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/stretchr/testify/suite"
)

// MigratorTestSuite extends FrameBaseTestSuite for migrator testing with real dependencies
type MigratorTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestMigratorCreation tests migrator creation with real dependencies
func (s *MigratorTestSuite) TestMigratorCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "migrator_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "migrator-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("MigratorCreation", func(t *testing.T) {
			// Test migrator creation
			migrator := NewMigrator(nil, "", nil)
			s.NotNil(migrator, "Should create migrator")
		})

		t.Run("MigratorInterfaceCompliance", func(t *testing.T) {
			// Test that our migrator implements the interface correctly
			var _ Migrator = NewMigrator(nil, "", nil)
		})
	})
}

// TestMigratorOperations tests migrator operations with real dependencies
func (s *MigratorTestSuite) TestMigratorOperations() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "migrator_operations_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "migrator-operations-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("MigratorLoadMigrations", func(t *testing.T) {
			// Test migrator load migrations
			migrator := NewMigrator(nil, "", nil)
			migrations, err := migrator.LoadMigrations(ctx)
			// With nil database and empty path, should handle gracefully
			s.Error(err, "Should fail with nil database")
			s.Nil(migrations, "Should return nil migrations on error")
		})

		t.Run("MigratorValidation", func(t *testing.T) {
			// Test migrator validation
			migrator := NewMigrator(nil, "", nil)
			err := migrator.ValidateMigrations(ctx)
			// With empty path, validation should handle gracefully
			s.Error(err, "Should fail validation with empty path")
		})
	})
}

func TestMigratorTestSuite(t *testing.T) {
	suite.Run(t, new(MigratorTestSuite))
}
