package datastore_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
)

// DatastoreTestSuite extends FrameBaseTestSuite for comprehensive datastore testing.
type DatastoreTestSuite struct {
	tests.BaseTestSuite
}

// TestDatastoreSuite runs the datastore test suite.
func TestDatastoreSuite(t *testing.T) {
	suite.Run(t, &DatastoreTestSuite{})
}

// TestServiceDatastore tests basic datastore functionality.
func (s *DatastoreTestSuite) TestServiceDatastore() {
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
func (s *DatastoreTestSuite) TestServiceDatastoreSet() {
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
func (s *DatastoreTestSuite) TestServiceDatastoreRunQuery() {
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
func (s *DatastoreTestSuite) TestServiceDatastoreRead() {
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
func (s *DatastoreTestSuite) TestServiceDatastoreNotSet() {
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
func (s *DatastoreTestSuite) TestDBPropertiesFromMap() {
	testCases := []struct {
		name     string
		propsMap map[string]any
		want     datastore.JSONMap
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
			want: datastore.JSONMap{
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
func (s *DatastoreTestSuite) TestDBPropertiesToMap() {
	testCases := []struct {
		name    string
		dbProps datastore.JSONMap
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
			dbProps: datastore.JSONMap{
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

// compareMapsByValue compares two maps only by their values, with special handling for numeric values.
func (s *DatastoreTestSuite) compareMapsByValue(map1, map2 map[string]any) bool {
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
func (s *DatastoreTestSuite) compareValues(val1, val2 any) bool {
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
func (s *DatastoreTestSuite) toFloat64(val any) (float64, bool) {
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
