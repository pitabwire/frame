package internal_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/internal"
)

// testStruct is a regular Go struct for JSON fallback testing.
type testStruct struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SerializerTestSuite provides comprehensive testing for the serializer functions.
func TestMarshal(t *testing.T) {
	testCases := []struct {
		name        string
		input       any
		expected    []byte
		expectError bool
	}{
		{
			name:        "nil input returns null",
			input:       nil,
			expected:    []byte("null"),
			expectError: false,
		},
		{
			name:        "empty byte slice",
			input:       []byte{},
			expected:    []byte{},
			expectError: false,
		},
		{
			name:        "byte slice with data",
			input:       []byte("hello world"),
			expected:    []byte("hello world"),
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    []byte(""),
			expectError: false,
		},
		{
			name:        "string with content",
			input:       "test string",
			expected:    []byte("test string"),
			expectError: false,
		},
		{
			name:        "json.RawMessage empty",
			input:       json.RawMessage{},
			expected:    []byte{},
			expectError: false,
		},
		{
			name:        "json.RawMessage with data",
			input:       json.RawMessage(`{"key":"value"}`),
			expected:    []byte(`{"key":"value"}`),
			expectError: false,
		},
		{
			name:        "regular JSONMap marshal",
			input:       data.JSONMap{"id": "sdfoaisdfasdf", "name": "test"},
			expected:    []byte(`{"id":"sdfoaisdfasdf","name":"test"}`),
			expectError: false,
		},
		{
			name: "structpb.Struct marshal",
			input: func() *structpb.Struct {
				s, _ := structpb.NewStruct(map[string]interface{}{
					"name":  "test",
					"value": 42,
					"nested": map[string]interface{}{
						"key": "nested_value",
					},
				})
				return s
			}(),
			expected:    nil, // StructPB marshal produces binary protobuf data
			expectError: false,
		},
		{
			name:        "map JSON marshal",
			input:       map[string]int{"a": 1, "b": 2},
			expected:    []byte(`{"a":1,"b":2}`),
			expectError: false,
		},
		{
			name:        "slice JSON marshal",
			input:       []string{"a", "b", "c"},
			expected:    []byte(`["a","b","c"]`),
			expectError: false,
		},
		{
			name:        "int JSON marshal",
			input:       42,
			expected:    []byte("42"),
			expectError: false,
		},
		{
			name:        "bool JSON marshal",
			input:       true,
			expected:    []byte("true"),
			expectError: false,
		},
		{
			name:        "float JSON marshal",
			input:       3.14,
			expected:    []byte("3.14"),
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := internal.Marshal(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.expected != nil {
				assert.Equal(t, tc.expected, result)
			} else if !tc.expectError {
				// For proto messages, just verify we get some data (would be JSON fallback)
				assert.NotNil(t, result)
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	testCases := []struct {
		name        string
		data        []byte
		holder      any
		setupHolder func() any
		verify      func(t *testing.T, holder any)
		expectError bool
		errorCheck  func(t *testing.T, err error)
	}{
		{
			name:        "nil holder error",
			data:        []byte("test"),
			holder:      nil,
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Equal(t, "holder is nil", err.Error())
			},
		},
		{
			name:        "non-pointer holder error",
			data:        []byte("test"),
			holder:      "not a pointer",
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Equal(t, "holder must be a non-nil pointer", err.Error())
			},
		},
		{
			name:        "nil pointer holder error",
			data:        []byte("test"),
			holder:      (*string)(nil),
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Equal(t, "holder must be a non-nil pointer", err.Error())
			},
		},
		{
			name: "byte slice zero copy",
			data: []byte("hello world"),
			setupHolder: func() any {
				var b []byte
				return &b
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*[]byte)
				assert.Equal(t, []byte("hello world"), result)
			},
			expectError: false,
		},
		{
			name: "byte slice reuse existing capacity",
			data: []byte("hi"),
			setupHolder: func() any {
				b := make([]byte, 0, 10)
				b = append(b, []byte("existing")...)
				return &b
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*[]byte)
				assert.Equal(t, []byte("hi"), result)
				assert.Equal(t, 10, cap(result)) // capacity should be preserved
			},
			expectError: false,
		},
		{
			name: "empty byte slice",
			data: []byte{},
			setupHolder: func() any {
				var b []byte
				return &b
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*[]byte)
				// Empty append results in nil slice, not empty slice
				assert.Nil(t, result)
			},
			expectError: false,
		},
		{
			name: "json.RawMessage zero copy",
			data: []byte(`{"key":"value"}`),
			setupHolder: func() any {
				var rm json.RawMessage
				return &rm
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*json.RawMessage)
				assert.JSONEq(t, `{"key":"value"}`, string(result))
			},
			expectError: false,
		},
		{
			name: "string from bytes",
			data: []byte("test string"),
			setupHolder: func() any {
				var s string
				return &s
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*string)
				assert.Equal(t, "test string", result)
			},
			expectError: false,
		},
		{
			name: "empty string",
			data: []byte{},
			setupHolder: func() any {
				var s string
				return &s
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*string)
				assert.Empty(t, result)
			},
			expectError: false,
		},
		{
			name: "struct JSONMap unmarshal",
			data: []byte(`{"id":"sdfoaisdfasdf","name":"test"}`),
			setupHolder: func() any {
				var jm data.JSONMap
				return &jm
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*data.JSONMap)
				assert.Equal(t, "sdfoaisdfasdf", result["id"])
				assert.Equal(t, "test", result["name"])
			},
			expectError: false,
		},
		{
			name: "structpb.Struct unmarshal",
			data: func() []byte {
				// Create a StructPB and marshal it to get binary protobuf data
				s, _ := structpb.NewStruct(map[string]interface{}{
					"name":  "test",
					"value": 42,
					"nested": map[string]interface{}{
						"key": "nested_value",
					},
				})
				data, _ := proto.Marshal(s)
				return data
			}(),
			setupHolder: func() any {
				return &structpb.Struct{}
			},
			verify: func(t *testing.T, holder any) {
				result := holder.(*structpb.Struct)
				assert.NotNil(t, result)
				// Verify the struct contains our test data
				fields := result.GetFields()
				assert.Equal(t, "test", fields["name"].GetStringValue())
				assert.InDelta(t, float64(42), fields["value"].GetNumberValue(), 0.001)
				nested := fields["nested"].GetStructValue()
				assert.Equal(t, "nested_value", nested.GetFields()["key"].GetStringValue())
			},
			expectError: false,
		},
		{
			name: "map JSON unmarshal",
			data: []byte(`{"a":1,"b":2}`),
			setupHolder: func() any {
				var m map[string]int
				return &m
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*map[string]int)
				expected := map[string]int{"a": 1, "b": 2}
				assert.Equal(t, expected, result)
			},
			expectError: false,
		},
		{
			name: "slice JSON unmarshal",
			data: []byte(`["x","y","z"]`),
			setupHolder: func() any {
				var s []string
				return &s
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*[]string)
				expected := []string{"x", "y", "z"}
				assert.Equal(t, expected, result)
			},
			expectError: false,
		},
		{
			name: "large data handling",
			data: func() []byte {
				// Create 1MB of data
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				return data
			}(),
			setupHolder: func() any {
				var b []byte
				return &b
			},
			verify: func(t *testing.T, holder any) {
				result := *holder.(*[]byte)
				assert.Len(t, result, 1024*1024)
				// Verify first few bytes
				for i := range 10 {
					assert.Equal(t, byte(i%256), result[i])
				}
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var holder any
			if tc.setupHolder != nil {
				holder = tc.setupHolder()
			} else {
				holder = tc.holder
			}

			err := internal.Unmarshal(tc.data, holder)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorCheck != nil {
					tc.errorCheck(t, err)
				}
				return
			}

			require.NoError(t, err)
			if tc.verify != nil {
				tc.verify(t, holder)
			}
		})
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	testCases := []struct {
		name  string
		value any
	}{
		{"byte slice", []byte("round trip test")},
		{"string", "round trip string"},
		{"json.RawMessage", json.RawMessage(`{"round":"trip"}`)},
		{"structpb.Struct", func() *structpb.Struct {
			s, _ := structpb.NewStruct(map[string]interface{}{
				"name":  "roundtrip",
				"value": 777,
			})
			return s
		}()},
		{"struct", testStruct{ID: 99, Name: "roundtrip"}},
		{"map", map[string]interface{}{"key": "value", "num": 42}},
		{"slice", []interface{}{"a", 1, true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Marshal
			data, err := internal.Marshal(tc.value)
			require.NoError(t, err)

			// Unmarshal back to same type
			var result interface{}
			switch tc.value.(type) {
			case []byte:
				var b []byte
				result = &b
			case string:
				var s string
				result = &s
			case json.RawMessage:
				var rm json.RawMessage
				result = &rm
			case *structpb.Struct:
				result = &structpb.Struct{}
			case testStruct:
				result = &testStruct{}
			case map[string]interface{}:
				var m map[string]interface{}
				result = &m
			case []interface{}:
				var s []interface{}
				result = &s
			}

			err = internal.Unmarshal(data, result)
			require.NoError(t, err)

			// Verify the round trip worked
			switch v := tc.value.(type) {
			case []byte:
				assert.Equal(t, v, *result.(*[]byte))
			case string:
				assert.Equal(t, v, *result.(*string))
			case json.RawMessage:
				assert.Equal(t, v, *result.(*json.RawMessage))
			case *structpb.Struct:
				original := v
				unmarshaled := result.(*structpb.Struct)
				// Compare the string representations since StructPB equality is complex
				assert.Equal(t, original.AsMap(), unmarshaled.AsMap())
			case testStruct:
				assert.Equal(t, v, *result.(*testStruct))
			case map[string]interface{}:
				expected := map[string]interface{}{"key": "value", "num": float64(42)}
				assert.Equal(t, expected, *result.(*map[string]interface{}))
			case []interface{}:
				expected := []interface{}{"a", float64(1), true}
				assert.Equal(t, expected, *result.(*[]interface{}))
			}
		})
	}
}
