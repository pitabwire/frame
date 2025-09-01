package frame_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pitabwire/frame"
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

				require.Equal(t, tc.serviceName, srv.Name(), "service name should match")

				w := srv.DB(ctx, false)
				require.NotNil(t, w, "write database should be instantiated")

				r := srv.DB(ctx, true)
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
				t.Setenv("DATABASE_URL", db.GetDS(ctx).String())

				defConf, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
				require.NoError(t, err, "configuration loading should succeed")

				defConf.DatabaseTraceQueries = tc.traceQueries

				ctx, srv := frame.NewService("Test Srv", frame.WithConfig(&defConf))
				srv.Init(ctx, frame.WithDatastore())

				w := srv.DB(ctx, false)
				require.NotNil(t, w, "write database should be available")

				r := srv.DB(ctx, true)
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
		expectError bool
	}{
		{
			name:        "run invalid query",
			query:       "SELECT 1 FROM",
			expectError: true,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				db := dep.ByIsDatabase(t.Context())

				t.Setenv("DATABASE_URL", db.GetDS(t.Context()).String())

				defConf, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
				require.NoError(t, err, "configuration loading should succeed")

				defConf.DatabaseTraceQueries = true

				ctx, srv := frame.NewService("Test Srv", frame.WithConfig(&defConf))
				srv.Init(ctx, frame.WithDatastore())

				w := srv.DB(ctx, false)
				require.NotNil(t, w, "write database should be available")

				r := srv.DB(ctx, true)
				require.NotNil(t, r, "read database should be available")

				_, err = w.Raw(tc.query).Rows()
				if tc.expectError {
					require.Error(t, err, "expected query to fail")
				} else {
					require.NoError(t, err, "query should succeed")
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

				ctx, srv := frame.NewService(tc.serviceName)

				mainDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), false)
				readDB := frame.WithDatastoreConnection(db.GetDS(ctx).String(), true)
				srv.Init(ctx, mainDB, readDB)

				w := srv.DB(ctx, false)
				require.NotNil(t, w, "write database should be available")

				r := srv.DB(ctx, true)
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
				ctx, srv := frame.NewService(tc.serviceName)

				w := srv.DB(ctx, false)
				require.Nil(t, w, "no database should be available when none is configured")
			})
		}
	})
}

// TestDBPropertiesFromMap tests conversion from string map to JSONMap.
func (s *DatastoreTestSuite) TestDBPropertiesFromMap() {
	testCases := []struct {
		name     string
		propsMap map[string]any
		want     frame.JSONMap
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
			want: frame.JSONMap{
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
		dbProps frame.JSONMap
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
			dbProps: frame.JSONMap{
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
