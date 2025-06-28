package frame

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// JSONMap is a GORM-compatible map[string]interface{} that stores JSONB/JSON in a DB.
type JSONMap map[string]interface{}

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
func (m *JSONMap) Scan(value interface{}) error {
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

	var temp map[string]interface{}
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
	var temp map[string]interface{}
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
		return clause.Expr{SQL: "?", Vars: []interface{}{nil}}
	}

	data, err := json.Marshal(*m)
	if err != nil {
		return clause.Expr{SQL: "?", Vars: []interface{}{nil}}
	}

	switch db.Dialector.Name() {
	case "mysql":
		return gorm.Expr("CAST(? AS JSON)", data)
	default:
		return gorm.Expr("?", data)
	}
}
