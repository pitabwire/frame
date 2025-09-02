package frame

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"maps"
	"sync"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// JSONMap is a GORM-compatible map[string]any that stores JSONB/JSON in a DB.
type JSONMap map[string]any

// bufferPool is used to minimize allocations for decoding.
//
//nolint:gochecknoglobals //optimization allows us to reuse byte buffers
var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// Value implements the driver.Valuer interface for database serialization.
func (m *JSONMap) Value() (driver.Value, error) {
	if m == nil || *m == nil {
		return nil, nil //nolint:nilnil //we don't need to error when there is nothing
	}
	// Avoid allocation by writing directly to JSON
	return json.Marshal(*m)
}

// Scan implements the sql.Scanner interface for database deserialization.
func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = make(JSONMap)
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("jsonmap: unsupported Scan type: %T", value)
	}

	buf, _ := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	_, _ = buf.Write(data)

	decoder := json.NewDecoder(buf)
	decoder.UseNumber()

	var temp map[string]any
	if err := decoder.Decode(&temp); err != nil {
		return fmt.Errorf("jsonmap: decode error: %w", err)
	}

	*m = temp
	return nil
}

// MarshalJSON customizes the JSON encoding.
func (m *JSONMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*m)
}

// UnmarshalJSON deserializes JSON into the map.
func (m *JSONMap) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*m = make(JSONMap)
		return nil
	}
	var temp map[string]any
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	*m = JSONMap(temp)
	return nil
}

// GormDataType returns the common GORM data type.
func (m *JSONMap) GormDataType() string {
	return "jsonmap"
}

// GormDBDataType returns the dialect-specific database column type.
func (m *JSONMap) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "JSONB"
	case "mysql", "sqlite":
		return "JSON"
	case "sqlserver":
		return "NVARCHAR(MAX)"
	default:
		return ""
	}
}

// GormValue optimizes how values are rendered in SQL for specific dialects.
func (m *JSONMap) GormValue(_ context.Context, db *gorm.DB) clause.Expr {
	if m == nil {
		return clause.Expr{SQL: "?", Vars: []any{nil}}
	}

	data, err := json.Marshal(*m)
	if err != nil {
		return clause.Expr{SQL: "?", Vars: []any{nil}}
	}

	switch db.Dialector.Name() {
	case "mysql":
		return gorm.Expr("CAST(? AS JSON)", data)
	default:
		return gorm.Expr("?", data)
	}
}

// ToProtoStruct converts a JSONMap into a structpb.Struct safely and efficiently.
func (m *JSONMap) ToProtoStruct() *structpb.Struct {
	if m == nil {
		return &structpb.Struct{Fields: make(map[string]*structpb.Value)}
	}

	refM := *m
	fields := make(map[string]*structpb.Value, len(refM))

	for k, v := range refM {
		// Validate UTF-8 keys (skip invalid ones)
		if !utf8.ValidString(k) {
			// Consider using structured logging instead of fmt.Printf in production
			fmt.Printf("ToProtoStruct: invalid UTF-8 in key %q\n", k)
			continue
		}

		// Convert values
		val, err := structpb.NewValue(v)
		if err != nil {
			// Skip unconvertible values instead of failing whole conversion
			fmt.Printf("ToProtoStruct: failed to convert key %q, value %+v: %v\n", k, v, err)
			continue
		}

		fields[k] = val
	}

	return &structpb.Struct{Fields: fields}
}

// FromProtoStruct populates the JSONMap with data from a protocol buffer Struct.
// If the receiver is nil, a new JSONMap will be created and returned.
// If the input struct is nil, the receiver is returned unchanged.
// Returns the receiver (or a new JSONMap if receiver was nil) for method chaining.
func (m *JSONMap) FromProtoStruct(s *structpb.Struct) *JSONMap {
	// Early return if no data to process
	if s == nil {
		return m
	}

	// Initialize receiver if nil
	if m == nil {
		m = &JSONMap{}
	}

	// Ensure map is initialized
	if *m == nil {
		*m = make(JSONMap)
	}

	// Safely convert protobuf struct to map and merge
	if srcMap := s.AsMap(); srcMap != nil {
		maps.Insert(*m, maps.All(srcMap))
	}

	return m
}
