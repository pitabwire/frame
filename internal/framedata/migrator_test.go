package framedata

import (
	"context"
	"testing"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
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
			migrator := NewMigrator(nil, nil, nil, nil)
			s.NotNil(migrator, "Should create migrator")
		})

		t.Run("MigratorInterfaceCompliance", func(t *testing.T) {
			// Test that our migrator implements the interface correctly
			var _ Migrator = NewMigrator(nil, nil, nil, nil)
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

		t.Run("MigratorGetPendingMigrations", func(t *testing.T) {
			// Test migrator get pending migrations
			migrator := NewMigrator(nil, nil, nil, nil)
			migrations, err := migrator.GetPendingMigrations(ctx)
			// With nil database, should handle gracefully
			s.Error(err, "Should fail with nil database")
			s.Nil(migrations, "Should return nil migrations on error")
		})

		t.Run("MigratorValidation", func(t *testing.T) {
			// Test migrator validation
			migrator := NewMigrator(nil, nil, nil, nil)
			err := migrator.ValidateMigrations(ctx)
			// With nil filesystem, validation should handle gracefully
			s.Error(err, "Should fail validation with nil filesystem")
		})
	})
}

func TestMigratorTestSuite(t *testing.T) {
	suite.Run(t, &MigratorTestSuite{
		FrameBaseTestSuite: frametests.FrameBaseTestSuite{
			InitResourceFunc: func(_ context.Context) []definition.TestResource {
				return []definition.TestResource{
					testpostgres.New(),
					testnats.New(),
				}
			},
		},
	})
}
