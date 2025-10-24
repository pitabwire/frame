package migration_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/migration"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
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
			migrationDir:  "./testdata/migrations/default",
			migrationPath: "./tests_runner/migrations/scans/scanned_select.sql",
			updateSQL:     "SELECT 2;",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, svc := frame.NewService(tc.serviceName, frame.WithDatastore(datastore.WithConnection(db.GetDS(t.Context()).String(), false)))

				svc.Init(ctx)

				dbMan := svc.DatastoreManager()
				dbPool := dbMan.GetPool(ctx, datastore.DefaultPoolName)
				// Clean up any existing migrations
				dbPool.DB(ctx, false).
					Session(&gorm.Session{AllowGlobalUpdate: true}).
					Unscoped().
					Delete(&migration.Migration{})

				// Apply initial migrations (this creates the migrations table)
				err := dbPool.Migrate(ctx, tc.migrationDir)
				require.NoError(t, err, "Initial migr should succeed")

				// Clean up specific migr for testing
				err = dbPool.DB(ctx, false).
					Where("name = ?", tc.migrationPath).
					Unscoped().
					Delete(&migration.Migration{}).
					Error
				require.NoError(t, err, "Cleanup of specific migr should succeed")

				// Read migr content
				migrationContent, err := os.ReadFile(tc.migrationPath)
				require.NoError(t, err, "Reading migr file should succeed")

				// Create migrator and save migr
				testMigrator := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB {
					return dbPool.DB(ctx, false)
				})

				err = testMigrator.SaveMigrationString(ctx, tc.migrationPath, string(migrationContent), "")
				require.NoError(t, err, "Saving new migr should succeed")

				// Verify migr was saved
				migr := migration.Migration{Name: tc.migrationPath}
				err = dbPool.DB(ctx, false).First(&migr, "name = ?", tc.migrationPath).Error
				require.NoError(t, err, "Finding saved migr should succeed")
				require.NotEmpty(t, migr.ID, "Migration ID should not be empty")

				// Update migr
				err = testMigrator.SaveMigrationString(ctx, tc.migrationPath, tc.updateSQL, "")
				require.NoError(t, err, "Updating migr should succeed")

				// Verify migr was updated
				updatedMigration := migration.Migration{Name: tc.migrationPath}
				err = dbPool.DB(ctx, false).First(&updatedMigration, "name = ?", tc.migrationPath).Error
				require.NoError(t, err, "Finding updated migr should succeed")
				require.Equal(t, migr.ID, updatedMigration.ID, "Migration IDs should match")
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
			migrationDir:       "./testdata/migrations/default",
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

				ctx, svc := frame.NewService(tc.serviceName, frame.WithConfig(&defConf), frame.WithDatastore(datastore.WithConnection(db.GetDS(t.Context()).String(), false)))

				svc.Init(ctx)

				dbMan := svc.DatastoreManager()
				require.NotNil(t, dbMan, "DatastoreManager should not be nil")

				dbPool := dbMan.GetPool(ctx, datastore.DefaultPoolName)
				require.NotNil(t, dbPool, "Database pool should not be nil")

				// Clean up existing migrations
				dbPool.DB(ctx, false).
					Session(&gorm.Session{AllowGlobalUpdate: true}).
					Unscoped().
					Delete(&migration.Migration{})

				// Apply migrations
				err = dbPool.Migrate(ctx, tc.migrationDir)
				require.NoError(t, err, "Migration application should succeed")

				// Verify migrations were applied
				var count int64
				err = dbPool.DB(ctx, false).Model(&migration.Migration{}).Count(&count).Error
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
			migrationDir: "./testdata/migrations/default",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, svc := frame.NewService(tc.serviceName, frame.WithDatastore(datastore.WithConnection(db.GetDS(t.Context()).String(), false)))

				svc.Init(ctx)

				dbMan := svc.DatastoreManager()
				dbPool := dbMan.GetPool(ctx, datastore.DefaultPoolName)

				// Clean up existing migrations
				dbPool.DB(ctx, false).
					Session(&gorm.Session{AllowGlobalUpdate: true}).
					Unscoped().
					Delete(&migration.Migration{})

				// Apply migrations
				err := dbPool.Migrate(ctx, tc.migrationDir)
				require.NoError(t, err, "Datastore migration should succeed")

				// Verify at least one migration exists
				var count int64
				err = dbPool.DB(ctx, false).Model(&migration.Migration{}).Count(&count).Error
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

				ctx, svc := frame.NewService(tc.serviceName, frame.WithDatastore(datastore.WithConnection(db.GetDS(t.Context()).String(), false)))

				svc.Init(ctx)

				dbMan := svc.DatastoreManager()
				dbPool := dbMan.GetPool(ctx, datastore.DefaultPoolName)
				// Clean up existing migrations
				dbPool.DB(ctx, false).
					Session(&gorm.Session{AllowGlobalUpdate: true}).
					Unscoped().
					Delete(&migration.Migration{})

				// Run migration multiple times to test idempotency
				var finalCount int64
				for i := range tc.runCount {
					err := dbPool.Migrate(ctx, tc.migrationDir)
					require.NoError(t, err, "Migration run %d should succeed", i+1)

					// Get count after each run
					err = dbPool.DB(ctx, false).Model(&migration.Migration{}).Count(&finalCount).Error
					require.NoError(t, err, "Counting migrations after run %d should succeed", i+1)
				}

				// Verify final count is consistent (idempotent)
				require.Positive(t, finalCount, "At least one migration should exist")

				// Run one more time and verify count doesn't change
				err := dbPool.Migrate(ctx, tc.migrationDir)
				require.NoError(t, err, "Final migration run should succeed")

				var finalCount2 int64
				err = dbPool.DB(ctx, false).Model(&migration.Migration{}).Count(&finalCount2).Error
				require.NoError(t, err, "Final count should succeed")
				require.Equal(t, finalCount, finalCount2, "Migration count should be idempotent")
			})
		}
	})
}
