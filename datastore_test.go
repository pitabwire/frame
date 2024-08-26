package frame_test

import (
	"github.com/pitabwire/frame"
	"gorm.io/datatypes"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func TestService_Datastore(t *testing.T) {

	testDBURL := frame.GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")

	ctx, srv := frame.NewService("Test Srv", frame.NoopDriver())

	mainDB := frame.DatastoreConnection(ctx, testDBURL, false)
	srv.Init(mainDB)

	if srv.Name() != "Test Srv" {
		t.Errorf("s")
	}

	w := srv.DB(ctx, false)
	if w == nil {
		t.Errorf("No default service could be instantiated")
		return
	}

	r := srv.DB(ctx, true)
	if r == nil {
		t.Errorf("Could not get read db instantiated")
		return
	}

	rd, _ := r.DB()
	if wd, _ := w.DB(); wd != rd {
		t.Errorf("Read and write db services should not be different ")
	}

	srv.Stop(ctx)

}

func TestService_DatastoreSet(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	var defConf frame.ConfigurationDefault
	err := frame.ConfigProcess("", &defConf)
	if err != nil {
		t.Errorf("Could not processFunc test configurations %v", err)
		return
	}
	ctx, srv := frame.NewService("Test Srv", frame.Config(&defConf))
	srv.Init(frame.Datastore(ctx))

	w := srv.DB(ctx, false)
	r := srv.DB(ctx, true)
	if w == nil || r == nil {
		t.Errorf("Read and write services setup but one couldn't be found")
		return
	}
}

func TestService_DatastoreRead(t *testing.T) {
	testDBURL := frame.GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")

	ctx, srv := frame.NewService("Test Srv")

	mainDB := frame.DatastoreConnection(ctx, testDBURL, false)
	readDB := frame.DatastoreConnection(ctx, testDBURL, true)
	srv.Init(mainDB, readDB)

	w := srv.DB(ctx, false)
	r := srv.DB(ctx, true)
	if w == nil || r == nil {
		t.Errorf("Read and write services setup but one couldn't be found")
		return
	}

	rd, _ := r.DB()
	wd, _ := w.DB()
	if wd == rd {
		t.Errorf("Read and write db services are same but we set different")
	}
}

func TestService_DatastoreNotSet(t *testing.T) {

	ctx, srv := frame.NewService("Test Srv")

	if w := srv.DB(ctx, false); w != nil {
		t.Errorf("When no connection is set no db is expected")
	}
}

func TestDBPropertiesFromMap(t *testing.T) {

	tests := []struct {
		name     string
		propsMap map[string]string
		want     datatypes.JSONMap
	}{
		{
			name: "happy case",
			propsMap: map[string]string{
				"a": "a",
				"b": "751",
				"c": "23.5",
				"d": "true",
				"e": "[23, 35, 37, 55]",
				"f": "{\"x\": \"t\", \"y\": \"g\" }",
			},
			want: datatypes.JSONMap{
				"a": "a",
				"b": "751",
				"c": "23.5",
				"d": "true",
				"e": []any{23, 35, 37, 55},
				"f": map[string]any{"x": "t", "y": "g"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frame.DBPropertiesFromMap(tt.propsMap); !compareMapsByValue(got, tt.want) {
				t.Errorf("DBPropertiesFromMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

// compareMapsByValue compares two maps only by their values, with special handling for numeric values.
func compareMapsByValue(map1, map2 map[string]interface{}) bool {
	if len(map1) != len(map2) {
		return false
	}

	for key, val1 := range map1 {
		val2, exists := map2[key]
		if !exists {
			return false
		}
		if !compareValues(val1, val2) {
			return false
		}
	}

	return true
}

// compareValues compares two interface{} values, handling basic types, slices, nested maps, and numeric comparisons.
func compareValues(val1, val2 interface{}) bool {

	// Handle other types including slices and nested maps
	switch v1 := val1.(type) {
	case string:
		return val1 == val2
	case []interface{}:
		v2, ok := val2.([]interface{})
		if !ok || len(v1) != len(v2) {
			return false
		}
		for i := range v1 {
			if !compareValues(v1[i], v2[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		v2, ok := val2.(map[string]interface{})
		if !ok {
			return false
		}
		return compareMapsByValue(v1, v2)
	default:

		f1, ok1 := toFloat64(val1)
		f2, ok2 := toFloat64(val2)
		if ok1 && ok2 {
			return f1 == f2
		}

		return reflect.DeepEqual(val1, val2)
	}
}

// toFloat64 attempts to convert an interface{} to float64, returning the value and whether it was successful.
func toFloat64(val interface{}) (float64, bool) {
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

func TestDBPropertiesToMap(t *testing.T) {
	tests := []struct {
		name    string
		dbProps datatypes.JSONMap
		want    map[string]string
	}{
		{
			name: "happy case",
			want: map[string]string{
				"a": "a",
				"b": "751",
				"c": "23.5",
				"d": "true",
				"e": "[23,35,37,55]",
				"f": "{\"x\":\"t\",\"y\":\"g\"}",
			},
			dbProps: datatypes.JSONMap{
				"a": "a",
				"b": "751",
				"c": "23.5",
				"d": "true",
				"e": []any{23, 35, 37, 55},
				"f": map[string]any{"x": "t", "y": "g"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frame.DBPropertiesToMap(tt.dbProps); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DBPropertiesToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
