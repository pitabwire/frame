package frame_test

import (
	"os"
	"testing"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// MigratorTestSuite extends BaseTestSuite for comprehensive migrator testing.
type MigratorTestSuite struct {
	tests.BaseTestSuite
}

// TestMigratorSuite runs the migrator test suite.
func TestMigratorSuite(t *testing.T) {
	suite.Run(t, &MigratorTestSuite{})
}

// TestSaveNewMigrations tests saving new migrations.
func (s *MigratorTestSuite) TestSaveNewMigrations() {
	testCases := []struct {
		name          string
		serviceName   string
		migrationDir  string
		migrationPath string
		updateSQL     string
	}{
		{
			name:          "save and update new migrations",
			serviceName:   "Test Migrations Srv",
			migrationDir:  "./tests_runner/migrations/default",
			migrationPath: "./tests_runner/migrations/scans/scanned_select.sql",
			updateSQL:     "SELECT 2;",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, svc := frame.NewService(tc.serviceName)

				mainDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), false)
				svc.Init(ctx, mainDB)

				// Clean up any existing migrations
				svc.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

				// Apply initial migrations
				err := svc.MigrateDatastore(ctx, tc.migrationDir)
				require.NoError(t, err, "Initial migration should succeed")

				// Clean up specific migration for testing
				err = svc.DB(ctx, false).Where("name = ?", tc.migrationPath).Unscoped().Delete(&frame.Migration{}).Error
				require.NoError(t, err, "Cleanup of specific migration should succeed")

				// Read migration content
				migrationContent, err := os.ReadFile(tc.migrationPath)
				require.NoError(t, err, "Reading migration file should succeed")

				// Create migrator and save migration
				pool := svc.DBPool()
				testMigrator := svc.NewMigrator(ctx, pool)

				err = testMigrator.SaveMigrationString(ctx, tc.migrationPath, string(migrationContent), "")
				require.NoError(t, err, "Saving new migration should succeed")

				// Verify migration was saved
				migration := frame.Migration{Name: tc.migrationPath}
				err = svc.DB(ctx, false).First(&migration, "name = ?", tc.migrationPath).Error
				require.NoError(t, err, "Finding saved migration should succeed")
				require.NotEmpty(t, migration.ID, "Migration ID should not be empty")

				// Update migration
				err = testMigrator.SaveMigrationString(ctx, tc.migrationPath, tc.updateSQL, "")
				require.NoError(t, err, "Updating migration should succeed")

				// Verify migration was updated
				updatedMigration := frame.Migration{Name: tc.migrationPath}
				err = svc.DB(ctx, false).First(&updatedMigration, "name = ?", tc.migrationPath).Error
				require.NoError(t, err, "Finding updated migration should succeed")
				require.Equal(t, migration.ID, updatedMigration.ID, "Migration IDs should match")
				require.Equal(t, tc.updateSQL, updatedMigration.Patch, "Migration patch should be updated")
			})
		}
	})
}

// TestApplyMigrations tests applying migrations.
func (s *MigratorTestSuite) TestApplyMigrations() {
	testCases := []struct {
		name               string
		serviceName        string
		migrationDir       string
		slowQueryThreshold string
		traceQueries       bool
		logLevel           string
	}{
		{
			name:               "apply migrations with configuration",
			serviceName:        "Test Migrations Srv",
			migrationDir:       "./tests_runner/migrations/default",
			slowQueryThreshold: "5ms",
			traceQueries:       true,
			logLevel:           "debug",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				defConf, err := config.FromEnv[config.ConfigurationDefault]()
				require.NoError(t, err, "Configuration loading should succeed")

				defConf.DatabaseSlowQueryLogThreshold = tc.slowQueryThreshold
				defConf.DatabaseTraceQueries = tc.traceQueries
				defConf.LogLevel = tc.logLevel

				ctx, svc := frame.NewService(tc.serviceName, frame.WithConfig(&defConf))

				mainDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), false)
				svc.Init(ctx, mainDB)

				// Clean up existing migrations
				svc.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

				// Apply migrations
				err = svc.MigrateDatastore(ctx, tc.migrationDir)
				require.NoError(t, err, "Migration application should succeed")

				// Verify migrations were applied
				var count int64
				err = svc.DB(ctx, false).Model(&frame.Migration{}).Count(&count).Error
				require.NoError(t, err, "Counting migrations should succeed")
				require.Positive(t, count, "At least one migration should exist")
			})
		}
	})
}

// TestServiceMigrateDatastore tests datastore migration.
func (s *MigratorTestSuite) TestServiceMigrateDatastore() {
	testCases := []struct {
		name         string
		serviceName  string
		migrationDir string
	}{
		{
			name:         "migrate datastore",
			serviceName:  "Test Migrations Srv",
			migrationDir: "./tests_runner/migrations/default",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, svc := frame.NewService(tc.serviceName)

				mainDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), false)
				svc.Init(ctx, mainDB)

				// Clean up existing migrations
				svc.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

				// Apply migrations
				err := svc.MigrateDatastore(ctx, tc.migrationDir)
				require.NoError(t, err, "Datastore migration should succeed")

				// Verify at least one migration exists
				var count int64
				err = svc.DB(ctx, false).Model(&frame.Migration{}).Count(&count).Error
				require.NoError(t, err, "Counting migrations should succeed")
				require.Positive(t, count, "At least one migration should exist")
			})
		}
	})
}

// TestServiceMigrateDatastoreIdempotency tests migration idempotency.
func (s *MigratorTestSuite) TestServiceMigrateDatastoreIdempotency() {
	testCases := []struct {
		name         string
		serviceName  string
		migrationDir string
		runCount     int
	}{
		{
			name:         "migrate datastore idempotency",
			serviceName:  "Test Migrations Srv",
			migrationDir: "./tests_runner/migrations/default",
			runCount:     3,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, svc := frame.NewService(tc.serviceName)

				mainDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), false)
				svc.Init(ctx, mainDB)

				// Clean up existing migrations
				svc.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

				// Run migration multiple times to test idempotency
				var finalCount int64
				for i := range tc.runCount {
					err := svc.MigrateDatastore(ctx, tc.migrationDir)
					require.NoError(t, err, "Migration run %d should succeed", i+1)

					// Get count after each run
					err = svc.DB(ctx, false).Model(&frame.Migration{}).Count(&finalCount).Error
					require.NoError(t, err, "Counting migrations after run %d should succeed", i+1)
				}

				// Verify final count is consistent (idempotent)
				require.Positive(t, finalCount, "At least one migration should exist")

				// Run one more time and verify count doesn't change
				err := svc.MigrateDatastore(ctx, tc.migrationDir)
				require.NoError(t, err, "Final migration run should succeed")

				var finalCount2 int64
				err = svc.DB(ctx, false).Model(&frame.Migration{}).Count(&finalCount2).Error
				require.NoError(t, err, "Final count should succeed")
				require.Equal(t, finalCount, finalCount2, "Migration count should be idempotent")
			})
		}
	})
}
