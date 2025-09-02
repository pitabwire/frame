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

// Copy returns a deep copy of the JSONMap.
// Nested maps and slices are recursively copied so that
// the returned JSONMap can be modified without affecting the original.
func (m *JSONMap) Copy() JSONMap {
	if m == nil || *m == nil {
		return JSONMap{}
	}

	out := make(JSONMap, len(*m))
	for k, v := range *m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue handles recursive deep-copying for JSON-compatible values.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, subv := range val {
			cp[k] = deepCopyValue(subv)
		}
		return cp

	case []any:
		cp := make([]any, len(val))
		for i, subv := range val {
			cp[i] = deepCopyValue(subv)
		}
		return cp

	// Primitive types are safe to reuse (immutable)
	case string, float64, bool, nil, json.Number:
		return val

	// If you expect other types, you can either clone them or fallback to JSON encode/decode.
	default:
		// fallback: encode-decode (slower but safe for unknown structs)
		data, _ := json.Marshal(val)
		var out any
		_ = json.Unmarshal(data, &out)
		return out
	}
}

// GetString retrieves a string value from the JSONMap by key.
// It returns the string and a boolean indicating if the value was found and is a string.
func (m *JSONMap) GetString(key string) string {
	if m == nil {
		return ""
	}

	val, ok := (*m)[key]
	if !ok {
		return ""
	}

	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// Update merges all key-value pairs from update into the receiver.
// If the receiver is nil, a new JSONMap is created.
// Keys in update overwrite existing keys in the receiver.
func (m *JSONMap) Update(update JSONMap) JSONMap {
	mCopy := m.Copy()

	if update == nil {
		return mCopy
	}

	// Copy update into existing map
	maps.Copy(mCopy, update)
	return mCopy
}

// ToProtoStruct converts a JSONMap into a structpb.Struct safely and efficiently.
func (m *JSONMap) ToProtoStruct() *structpb.Struct {
	if m == nil {
		return &structpb.Struct{Fields: make(map[string]*structpb.Value)}
	}

	fields := make(map[string]*structpb.Value, len(*m))

	for k, v := range *m {
		// Validate UTF-8 keys (skip invalid ones)
		if !utf8.ValidString(k) {
			continue
		}

		// Convert values
		val, err := structpb.NewValue(v)
		if err != nil {
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
func (m *JSONMap) FromProtoStruct(s *structpb.Struct) JSONMap {
	mCopy := m.Copy()

	// Early return if no data to process
	if s == nil {
		return mCopy
	}

	structMap := s.AsMap()
	// Safely convert protobuf struct to map and merge
	if structMap == nil {
		return mCopy
	}

	maps.Copy(mCopy, structMap)
	return mCopy
}
