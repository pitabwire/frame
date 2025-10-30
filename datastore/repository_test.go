package datastore_test

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
)

// RepositoryTestSuite extends FrameBaseTestSuite for comprehensive datastore testing.
type RepositoryTestSuite struct {
	tests.BaseTestSuite
}

// TestDatastoreSuite runs the datastore test suite.
func TestDatastoreSuite(t *testing.T) {
	suite.Run(t, &RepositoryTestSuite{})
}

// TestServiceDatastore tests basic datastore functionality.
func (s *RepositoryTestSuite) TestServiceDatastore() {
	testCases := []struct {
		name        string
		serviceName string
		expectError bool
	}{
		{
			name:        "basic datastore setup",
			serviceName: "Test Srv",
			expectError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, srv := frame.NewService(tc.serviceName, frametests.WithNoopDriver())

				mainDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), false)
				srv.Init(ctx, mainDB)

				require.Equal(t, tc.serviceName, srv.Name(), "dbPool name should match")

				dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				w := dbPool.DB(ctx, false)
				require.NotNil(t, w, "write database should be instantiated")

				r := dbPool.DB(ctx, true)
				require.NotNil(t, r, "read database should be instantiated")

				rd, _ := r.DB()
				wd, _ := w.DB()
				require.Equal(t, wd, rd, "read and write db services should be the same for single connection")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceDatastoreSet tests datastore setup with configuration.
func (s *RepositoryTestSuite) TestServiceDatastoreSet() {
	testCases := []struct {
		name         string
		envVars      map[string]string
		traceQueries bool
		expectError  bool
	}{
		{
			name:         "datastore with environment configuration",
			traceQueries: true,
			expectError:  false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				db := dep.ByIsDatabase(ctx)
				// Set environment variables
				dbURL := db.GetDS(ctx).String()
				t.Setenv("DATABASE_URL", dbURL)

				defConf, err := config.FromEnv[config.ConfigurationDefault]()
				require.NoError(t, err, "configuration loading should succeed")

				defConf.DatabaseTraceQueries = tc.traceQueries

				ctx, srv := frame.NewService("Test Srv", frame.WithConfig(&defConf), frame.WithDatastore())
				srv.Init(ctx)

				dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				w := dbPool.DB(ctx, false)
				require.NotNil(t, w, "write database should be available")

				r := dbPool.DB(ctx, true)
				require.NotNil(t, r, "read database should be available")
			})
		}
	})
}

// TestServiceDatastoreRunQuery tests query execution.
func (s *RepositoryTestSuite) TestServiceDatastoreRunQuery() {
	testCases := []struct {
		name        string
		query       string
		expectError require.ErrorAssertionFunc
	}{
		{
			name:        "run invalid query",
			query:       "SELECT 1 FROM",
			expectError: require.Error,
		},
		{
			name:        "run invalid query",
			query:       "SELECT 1 ;",
			expectError: require.NoError,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				t.Setenv("DATABASE_URL", db.GetDS(t.Context()).String())

				defConf, err := config.FromEnv[config.ConfigurationDefault]()
				require.NoError(t, err, "configuration loading should succeed")

				defConf.DatabaseTraceQueries = true

				ctx, svc := frame.NewService(
					"Test Srv",
					frame.WithConfig(&defConf),
					frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
				)

				svc.Init(ctx)

				dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				w := dbPool.DB(ctx, false)
				require.NotNil(t, w, "write database should be available")

				r := dbPool.DB(ctx, true)
				require.NotNil(t, r, "read database should be available")

				rows, err := w.Raw(tc.query).Rows()
				tc.expectError(t, err, "error assertion on query failed")

				if rows != nil {
					err = rows.Err()
					tc.expectError(t, err, "error assertion on query failed")
					util.CloseAndLogOnError(ctx, rows, "couldn't close rows")
				}
			})
		}
	})
}

// TestServiceDatastoreRead tests separate read/write datastores.
func (s *RepositoryTestSuite) TestServiceDatastoreRead() {
	testCases := []struct {
		name        string
		serviceName string
		expectError bool
	}{
		{
			name:        "separate read/write datastores",
			serviceName: "Test Srv",
			expectError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				ctx, srv := frame.NewService(
					tc.serviceName,
					frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
				)

				readDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), true)
				srv.Init(ctx, readDB)

				dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				w := dbPool.DB(ctx, false)
				require.NotNil(t, w, "write database should be available")

				r := dbPool.DB(ctx, true)
				require.NotNil(t, r, "read database should be available")

				rd, _ := r.DB()
				wd, _ := w.DB()
				require.NotEqual(
					t,
					wd,
					rd,
					"read and write db services should be different when separate connections are set",
				)
			})
		}
	})
}

// TestServiceDatastoreNotSet tests behavior when no datastore is configured.
func (s *RepositoryTestSuite) TestServiceDatastoreNotSet() {
	testCases := []struct {
		name        string
		serviceName string
	}{
		{
			name:        "no datastore configured",
			serviceName: "Test Srv",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(tc.serviceName)

				require.Nil(
					t,
					srv.DatastoreManager(),
					"no database manager should be available when none is configured",
				)
			})
		}
	})
}

// TestDBPropertiesFromMap tests conversion from string map to JSONMap.
func (s *RepositoryTestSuite) TestDBPropertiesFromMap() {
	testCases := []struct {
		name     string
		propsMap map[string]any
		want     data.JSONMap
	}{
		{
			name: "happy case with various data types",
			propsMap: map[string]any{
				"a": "a",
				"b": "751",
				"c": "23.5",
				"d": "true",
				"e": []any{23, 35, 37, 55},
				"f": map[string]any{"x": "t", "y": "g"},
			},
			want: data.JSONMap{
				"a": "a",
				"b": "751",
				"c": "23.5",
				"d": "true",
				"e": []any{23, 35, 37, 55},
				"f": map[string]any{"x": "t", "y": "g"},
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got, _ := structpb.NewStruct(tc.propsMap)
				require.True(
					t,
					s.compareMapsByValue(got.AsMap(), tc.want),
					"DBPropertiesFromMap result should match expected",
				)
			})
		}
	})
}

// TestDBPropertiesToMap tests conversion from JSONMap to string map.
func (s *RepositoryTestSuite) TestDBPropertiesToMap() {
	testCases := []struct {
		name    string
		dbProps data.JSONMap
		want    map[string]any
	}{
		{
			name: "happy case with various data types",
			want: map[string]any{
				"a": "a",
				"b": 751.0,
				"c": "23.5",
				"d": "true",
				"e": []any{23.0, 35.0, 37.0, 55.0},
				"f": map[string]any{"x": "t", "y": "g"},
			},
			dbProps: data.JSONMap{
				"a": "a",
				"b": 751,
				"c": "23.5",
				"d": "true",
				"e": []any{23, 35, 37, 55},
				"f": map[string]any{"x": "t", "y": "g"},
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got, _ := structpb.NewStruct(tc.dbProps)
				require.Equal(t, tc.want, got.AsMap(), "DBPropertiesToMap result should match expected")
			})
		}
	})
}

// TestEntity is a test model for repository testing.
type TestEntity struct {
	data.BaseModel
	Name        string `gorm:"type:varchar(255)"`
	Description string `gorm:"type:text"`
	Status      string `gorm:"type:varchar(50);default:'pending'"`
	Counter     int    `gorm:"default:0"`
}

// TestCreate tests the Create function with various scenarios.
func (s *RepositoryTestSuite) TestCreate() {
	testCases := []struct {
		name        string
		setupEntity func(ctx context.Context) *TestEntity
		expectError bool
		errorMsg    string
	}{
		{
			name: "create valid entity",
			setupEntity: func(ctx context.Context) *TestEntity {
				return &TestEntity{
					Name:        "Test Entity",
					Description: "Test Description",
					Counter:     1,
				}
			},
			expectError: false,
		},
		{
			name: "create entity with pre-set ID",
			setupEntity: func(ctx context.Context) *TestEntity {
				entity := &TestEntity{
					Name: "Entity with ID",
				}
				// Use unique timestamp-based ID to avoid conflicts between test runs
				entity.ID = fmt.Sprintf("custom-id-%d", time.Now().UnixNano())
				return entity
			},
			expectError: false,
		},
		{
			name: "create entity with version > 0 should fail",
			setupEntity: func(ctx context.Context) *TestEntity {
				entity := &TestEntity{
					Name: "Invalid Version",
				}
				entity.Version = 5
				return entity
			},
			expectError: true,
			errorMsg:    "version is more than 0",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				db := dep.ByIsDatabase(ctx)

				ctx, srv := frame.NewService(
					"Test Create Service",
					frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
				)
				srv.Init(ctx)
				defer srv.Stop(ctx)

				dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
				require.NoError(t, err, "auto-migrate should succeed")

				repo := datastore.NewBaseRepository[*TestEntity](
					ctx,
					dbPool,
					srv.WorkManager(),
					func() *TestEntity { return &TestEntity{} },
				)

				entity := tc.setupEntity(ctx)
				err = repo.Create(ctx, entity)

				if tc.expectError {
					require.Error(t, err)
					if tc.errorMsg != "" {
						require.Contains(t, err.Error(), tc.errorMsg)
					}
				} else {
					require.NoError(t, err, "create should succeed")
					require.NotEmpty(t, entity.GetID(), "entity should have an ID")
					require.Equal(t, uint(1), entity.GetVersion(), "version should be 1 after create")
					require.NotZero(t, entity.CreatedAt, "created_at should be set")
					require.NotZero(t, entity.ModifiedAt, "modified_at should be set")

					// Verify entity was saved to database
					fetched, err := repo.GetByID(ctx, entity.GetID())
					require.NoError(t, err)
					require.Equal(t, entity.Name, fetched.Name)
					require.Equal(t, entity.Description, fetched.Description)
				}
			})
		}
	})
}

// TestImmutableFields tests that immutable fields cannot be modified.
func (s *RepositoryTestSuite) TestImmutableFields() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		ctx := t.Context()
		db := dep.ByIsDatabase(ctx)

		ctx, srv := frame.NewService(
			"Test Immutable Service",
			frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
		)
		srv.Init(ctx)
		defer srv.Stop(ctx)

		dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
		require.NoError(t, err)

		repo := datastore.NewBaseRepository[*TestEntity](
			ctx,
			dbPool,
			srv.WorkManager(),
			func() *TestEntity { return &TestEntity{} },
		)

		// Verify immutable fields are set
		immutableFields := repo.ImmutableFields()
		require.NotEmpty(t, immutableFields, "immutable fields should be defined")
		require.Contains(t, immutableFields, "id", "id should be immutable")
		require.Contains(t, immutableFields, "created_at", "created_at should be immutable")
		require.Contains(t, immutableFields, "tenant_id", "tenant_id should be immutable")
		require.Contains(t, immutableFields, "partition_id", "partition_id should be immutable")

		// Create entity
		entity := &TestEntity{
			Name:    "Original Name",
			Counter: 1,
		}
		err = repo.Create(ctx, entity)
		require.NoError(t, err)

		originalID := entity.GetID()
		originalCreatedAt := entity.CreatedAt
		originalTenantID := entity.TenantID

		// Try to update entity - attempt to change immutable fields (they should be ignored)
		entity.Name = "Updated Name"
		// Note: We DON'T change entity.ID because it's needed for the WHERE clause
		// The Omit() in Update should prevent created_at, tenant_id from being updated
		entity.CreatedAt = time.Now().Add(24 * time.Hour)
		entity.TenantID = "new-tenant-should-not-update"

		// Update without specifying fields (should omit immutable fields)
		rowsAffected, err := repo.Update(ctx, entity)
		require.NoError(t, err)
		require.Equal(t, int64(1), rowsAffected, "should update 1 row")

		// Fetch and verify immutable fields unchanged
		updated, err := repo.GetByID(ctx, originalID)
		require.NoError(t, err)
		require.Equal(t, originalID, updated.GetID(), "ID should not change")
		// PostgreSQL truncates to microseconds, so use WithinDuration for timestamp comparison
		require.WithinDuration(t, originalCreatedAt, updated.CreatedAt, time.Microsecond, "created_at should not change")
		require.Equal(t, originalTenantID, updated.TenantID, "tenant_id should not change")
		require.Equal(t, "Updated Name", updated.Name, "name should be updated")
	})
}

// TestBulkCreate tests bulk creation of entities.
func (s *RepositoryTestSuite) TestBulkCreate() {
	testCases := []struct {
		name        string
		entityCount int
		expectError bool
	}{
		{
			name:        "bulk create 10 entities",
			entityCount: 10,
			expectError: false,
		},
		{
			name:        "bulk create 100 entities",
			entityCount: 100,
			expectError: false,
		},
		{
			name:        "bulk create 1000 entities",
			entityCount: 1000,
			expectError: false,
		},
		{
			name:        "bulk create empty slice",
			entityCount: 0,
			expectError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				db := dep.ByIsDatabase(ctx)

				ctx, srv := frame.NewService(
					"Test Bulk Create Service",
					frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
				)
				srv.Init(ctx)
				defer srv.Stop(ctx)

				dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
				require.NoError(t, err)

				repo := datastore.NewBaseRepository[*TestEntity](
					ctx,
					dbPool,
					srv.WorkManager(),
					func() *TestEntity { return &TestEntity{} },
				)

				// Create entities
				entities := make([]*TestEntity, tc.entityCount)
				for i := 0; i < tc.entityCount; i++ {
					entities[i] = &TestEntity{
						Name:        fmt.Sprintf("Entity-%d", i),
						Description: fmt.Sprintf("Description for entity %d", i),
						Counter:     i,
						Status:      "pending",
					}
				}

				start := time.Now()
				err = repo.BulkCreate(ctx, entities)
				duration := time.Since(start)

				if tc.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err, "bulk create should succeed")

					if tc.entityCount > 0 {
						t.Logf("Bulk created %d entities in %v (%.2f entities/sec)",
							tc.entityCount, duration, float64(tc.entityCount)/duration.Seconds())

						// Verify all entities were created
						for i, entity := range entities {
							require.NotEmpty(t, entity.GetID(), "entity %d should have an ID", i)
							require.Equal(t, uint(1), entity.GetVersion(), "entity %d should have version 1", i)
						}

						// Spot check: verify first and last entities in database
						first, err := repo.GetByID(ctx, entities[0].GetID())
						require.NoError(t, err)
						require.Equal(t, "Entity-0", first.Name)

						last, err := repo.GetByID(ctx, entities[tc.entityCount-1].GetID())
						require.NoError(t, err)
						require.Equal(t, fmt.Sprintf("Entity-%d", tc.entityCount-1), last.Name)

						// Verify count
						count, err := repo.Count(ctx)
						require.NoError(t, err)
						require.GreaterOrEqual(t, count, int64(tc.entityCount), "count should include created entities")
					}
				}
			})
		}
	})
}

// TestBulkCreateBatchSize tests that bulk create respects batch size.
func (s *RepositoryTestSuite) TestBulkCreateBatchSize() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		ctx := t.Context()
		db := dep.ByIsDatabase(ctx)

		ctx, srv := frame.NewService(
			"Test Batch Size Service",
			frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
		)
		srv.Init(ctx)
		defer srv.Stop(ctx)

		dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
		require.NoError(t, err)

		repo := datastore.NewBaseRepository[*TestEntity](
			ctx,
			dbPool,
			srv.WorkManager(),
			func() *TestEntity { return &TestEntity{} },
		)

		batchSize := repo.BatchSize()
		require.Greater(t, batchSize, 0, "batch size should be positive")
		t.Logf("Repository batch size: %d", batchSize)

		// Create more entities than batch size to test batching
		entityCount := batchSize * 2
		entities := make([]*TestEntity, entityCount)
		for i := 0; i < entityCount; i++ {
			entities[i] = &TestEntity{
				Name:    fmt.Sprintf("Batch-Entity-%d", i),
				Counter: i,
			}
		}

		err = repo.BulkCreate(ctx, entities)
		require.NoError(t, err)

		// Verify all entities were created
		count, err := repo.Count(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, count, int64(entityCount))
	})
}

// TestBulkUpdate tests bulk update functionality.
func (s *RepositoryTestSuite) TestBulkUpdate() {
	testCases := []struct {
		name         string
		entityCount  int
		updateParams map[string]any
		expectError  bool
	}{
		{
			name:        "bulk update status field",
			entityCount: 50,
			updateParams: map[string]any{
				"status": "completed",
			},
			expectError: false,
		},
		{
			name:        "bulk update multiple fields",
			entityCount: 25,
			updateParams: map[string]any{
				"status":      "active",
				"counter":     999,
				"description": "Bulk updated description",
			},
			expectError: false,
		},
		{
			name:         "bulk update with empty params should fail",
			entityCount:  10,
			updateParams: map[string]any{},
			expectError:  true,
		},
		{
			name:        "bulk update with empty entity list",
			entityCount: 0,
			updateParams: map[string]any{
				"status": "active",
			},
			expectError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				db := dep.ByIsDatabase(ctx)

				ctx, srv := frame.NewService(
					"Test Bulk Update Service",
					frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
				)
				srv.Init(ctx)
				defer srv.Stop(ctx)

				dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
				err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
				require.NoError(t, err)

				repo := datastore.NewBaseRepository[*TestEntity](
					ctx,
					dbPool,
					srv.WorkManager(),
					func() *TestEntity { return &TestEntity{} },
				)

				// Create entities to update
				entities := make([]*TestEntity, tc.entityCount)
				entityIDs := make([]string, tc.entityCount)

				for i := 0; i < tc.entityCount; i++ {
					entities[i] = &TestEntity{
						Name:        fmt.Sprintf("Entity-%d", i),
						Description: "Original description",
						Status:      "pending",
						Counter:     i,
					}
				}

				if tc.entityCount > 0 {
					err = repo.BulkCreate(ctx, entities)
					require.NoError(t, err)

					for i, entity := range entities {
						entityIDs[i] = entity.GetID()
					}
				}

				// Perform bulk update
				start := time.Now()
				rowsAffected, err := repo.BulkUpdate(ctx, entityIDs, tc.updateParams)
				duration := time.Since(start)

				if tc.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err, "bulk update should succeed")
					require.Equal(t, int64(tc.entityCount), rowsAffected, "should update all entities")

					if tc.entityCount > 0 {
						t.Logf("Bulk updated %d entities in %v (%.2f entities/sec)",
							tc.entityCount, duration, float64(tc.entityCount)/duration.Seconds())

						// Verify updates
						for i, entityID := range entityIDs {
							updated, err := repo.GetByID(ctx, entityID)
							require.NoError(t, err, "entity %d should be retrievable", i)

							// Verify each updated field
							for key, expectedValue := range tc.updateParams {
								switch key {
								case "status":
									require.Equal(t, expectedValue, updated.Status, "status should be updated")
								case "counter":
									require.Equal(t, expectedValue, updated.Counter, "counter should be updated")
								case "description":
									require.Equal(t, expectedValue, updated.Description, "description should be updated")
								}
							}

							// Verify name was not changed (not in update params)
							require.Equal(t, fmt.Sprintf("Entity-%d", i), updated.Name, "name should not change")
						}
					}
				}
			})
		}
	})
}

// TestBulkUpdateInvalidColumn tests that bulk update validates column names.
func (s *RepositoryTestSuite) TestBulkUpdateInvalidColumn() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		ctx := t.Context()
		db := dep.ByIsDatabase(ctx)

		ctx, srv := frame.NewService(
			"Test Invalid Column Service",
			frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
		)
		srv.Init(ctx)
		defer srv.Stop(ctx)

		dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
		require.NoError(t, err)

		repo := datastore.NewBaseRepository[*TestEntity](
			ctx,
			dbPool,
			srv.WorkManager(),
			func() *TestEntity { return &TestEntity{} },
		)

		// Create a test entity
		entity := &TestEntity{Name: "Test Entity"}
		err = repo.Create(ctx, entity)
		require.NoError(t, err)

		// Try to update with invalid column name
		invalidParams := map[string]any{
			"invalid_column_name": "some value",
		}

		rowsAffected, err := repo.BulkUpdate(ctx, []string{entity.GetID()}, invalidParams)
		require.Error(t, err, "should fail with invalid column name")
		require.Contains(t, err.Error(), "invalid column name")
		require.Equal(t, int64(0), rowsAffected)
	})
}

// TestBulkUpdateConcurrent tests concurrent bulk updates.
func (s *RepositoryTestSuite) TestBulkUpdateConcurrent() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		ctx := t.Context()
		db := dep.ByIsDatabase(ctx)

		ctx, srv := frame.NewService(
			"Test Concurrent Bulk Service",
			frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
		)
		srv.Init(ctx)
		defer srv.Stop(ctx)

		dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
		require.NoError(t, err)

		repo := datastore.NewBaseRepository[*TestEntity](
			ctx,
			dbPool,
			srv.WorkManager(),
			func() *TestEntity { return &TestEntity{} },
		)

		// Create entities
		entityCount := 100
		entities := make([]*TestEntity, entityCount)
		for i := 0; i < entityCount; i++ {
			entities[i] = &TestEntity{
				Name:    fmt.Sprintf("Concurrent-Entity-%d", i),
				Counter: 0,
				Status:  "pending",
			}
		}

		err = repo.BulkCreate(ctx, entities)
		require.NoError(t, err)

		// Divide entities into groups for concurrent updates
		numGroups := 5
		entitiesPerGroup := entityCount / numGroups
		var wg sync.WaitGroup
		errors := make([]error, numGroups)

		for g := 0; g < numGroups; g++ {
			wg.Add(1)
			go func(groupIdx int) {
				defer wg.Done()

				start := groupIdx * entitiesPerGroup
				groupIDs := make([]string, entitiesPerGroup)

				for i := 0; i < entitiesPerGroup; i++ {
					groupIDs[i] = entities[start+i].GetID()
				}

				params := map[string]any{
					"status":  fmt.Sprintf("group-%d", groupIdx),
					"counter": groupIdx * 100,
				}

				_, err := repo.BulkUpdate(ctx, groupIDs, params)
				errors[groupIdx] = err
			}(g)
		}

		wg.Wait()

		// Verify all updates succeeded
		for i, err := range errors {
			require.NoError(t, err, "group %d update should succeed", i)
		}

		// Verify updates were applied
		for g := 0; g < numGroups; g++ {
			start := g * entitiesPerGroup
			firstEntityInGroup, err := repo.GetByID(ctx, entities[start].GetID())
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("group-%d", g), firstEntityInGroup.Status)
			require.Equal(t, g*100, firstEntityInGroup.Counter)
		}
	})
}

// TestBulkUpdatePerformance tests performance characteristics of bulk updates.
func (s *RepositoryTestSuite) TestBulkUpdatePerformance() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		ctx := t.Context()
		db := dep.ByIsDatabase(ctx)

		ctx, srv := frame.NewService(
			"Test Performance Service",
			frame.WithDatastore(pool.WithConnection(db.GetDS(t.Context()).String(), false)),
		)
		srv.Init(ctx)
		defer srv.Stop(ctx)

		dbPool := srv.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		err := dbPool.DB(ctx, false).AutoMigrate(&TestEntity{})
		require.NoError(t, err)

		repo := datastore.NewBaseRepository[*TestEntity](
			ctx,
			dbPool,
			srv.WorkManager(),
			func() *TestEntity { return &TestEntity{} },
		)

		// Create a large number of entities
		entityCount := 5000
		entities := make([]*TestEntity, entityCount)
		entityIDs := make([]string, entityCount)

		for i := 0; i < entityCount; i++ {
			entities[i] = &TestEntity{
				Name:    fmt.Sprintf("Perf-Entity-%d", i),
				Status:  "initial",
				Counter: 0,
			}
		}

		// Measure bulk create performance
		createStart := time.Now()
		err = repo.BulkCreate(ctx, entities)
		createDuration := time.Since(createStart)
		require.NoError(t, err)

		t.Logf("Bulk created %d entities in %v (%.2f entities/sec)",
			entityCount, createDuration, float64(entityCount)/createDuration.Seconds())

		for i, entity := range entities {
			entityIDs[i] = entity.GetID()
		}

		// Measure bulk update performance
		updateParams := map[string]any{
			"status":  "updated",
			"counter": 999,
		}

		updateStart := time.Now()
		rowsAffected, err := repo.BulkUpdate(ctx, entityIDs, updateParams)
		updateDuration := time.Since(updateStart)

		require.NoError(t, err)
		require.Equal(t, int64(entityCount), rowsAffected)

		t.Logf("Bulk updated %d entities in %v (%.2f entities/sec)",
			entityCount, updateDuration, float64(entityCount)/updateDuration.Seconds())

		// Performance assertions - bulk operations should be fast
		require.Less(t, createDuration.Seconds(), 30.0, "bulk create should complete within 30 seconds")
		require.Less(t, updateDuration.Seconds(), 10.0, "bulk update should complete within 10 seconds")

		// Spot check updates
		firstUpdated, err := repo.GetByID(ctx, entityIDs[0])
		require.NoError(t, err)
		require.Equal(t, "updated", firstUpdated.Status)
		require.Equal(t, 999, firstUpdated.Counter)

		lastUpdated, err := repo.GetByID(ctx, entityIDs[entityCount-1])
		require.NoError(t, err)
		require.Equal(t, "updated", lastUpdated.Status)
		require.Equal(t, 999, lastUpdated.Counter)
	})
}

// compareMapsByValue compares two maps only by their values, with special handling for numeric values.
func (s *RepositoryTestSuite) compareMapsByValue(map1, map2 map[string]any) bool {
	if len(map1) != len(map2) {
		return false
	}

	for key, val1 := range map1 {
		val2, exists := map2[key]
		if !exists {
			return false
		}
		if !s.compareValues(val1, val2) {
			return false
		}
	}

	return true
}

// compareValues compares two any values, handling basic types, slices, nested maps, and numeric comparisons.
func (s *RepositoryTestSuite) compareValues(val1, val2 any) bool {
	// Handle other types including slices and nested maps
	switch v1 := val1.(type) {
	case string:
		return val1 == val2
	case []any:
		v2, ok := val2.([]any)
		if !ok || len(v1) != len(v2) {
			return false
		}
		for i := range v1 {
			if !s.compareValues(v1[i], v2[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		v2, ok := val2.(map[string]any)
		if !ok {
			return false
		}
		return s.compareMapsByValue(v1, v2)
	default:
		f1, ok1 := s.toFloat64(val1)
		f2, ok2 := s.toFloat64(val2)
		if ok1 && ok2 {
			return f1 == f2
		}

		return reflect.DeepEqual(val1, val2)
	}
}

// toFloat64 attempts to convert an any to float64, returning the value and whether it was successful.
func (s *RepositoryTestSuite) toFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
