package tenancy

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

// ErrEnrollmentStrict is returned when strict enrollment detects a model
// that looks tenanted but does not implement Tenanted and is not Unscoped.
var ErrEnrollmentStrict = errors.New(
	"tenancy: enrollment strict: model looks tenanted but does not implement Tenanted",
)

// EnrolledModels filters the supplied migration models, returning
// ModelInfo for those that satisfy the Tenanted interface and do NOT
// satisfy Unscoped. Tenant and partition column names default to the
// conventional "tenant_id" / "partition_id"; future overrides can come
// from per-model tags but are not required today.
//
// The supplied *gorm.DB is used only as a statement context for table
// name resolution — no queries are executed. GORM's statement parser
// honours any TableName() string method or gorm struct tags on the
// model, so custom table-name overrides are respected.
func EnrolledModels(db *gorm.DB, models []any) ([]ModelInfo, error) {
	return EnrolledModelsWithOptions(db, models, false)
}

// EnrolledModelsWithOptions is EnrolledModels with optional strict detection.
// When strict is true, models that look tenanted (TenantID field, tenant_id
// gorm tag, or embedded BaseModel) but do not implement Tenanted and are
// not Unscoped cause an error.
func EnrolledModelsWithOptions(db *gorm.DB, models []any, strict bool) ([]ModelInfo, error) {
	if len(models) == 0 {
		return nil, nil
	}
	enrolled := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		if m == nil {
			continue
		}
		// Tenanted/Unscoped are declared with pointer receivers, so a
		// model registered BY VALUE silently failed both assertions and
		// was skipped — leaving its table without RLS while everything
		// else worked. Four services shipped that way (2026-06 audit).
		// Promote values to pointers before checking so registration
		// style cannot change enforcement.
		m = asPointer(m)
		if _, isUnscoped := m.(Unscoped); isUnscoped {
			continue
		}
		if _, isTenanted := m.(Tenanted); !isTenanted {
			if strict && looksTenanted(m) {
				return nil, fmt.Errorf("%w: %T", ErrEnrollmentStrict, m)
			}
			continue
		}
		table, err := tableNameFor(db, m)
		if err != nil {
			return nil, err
		}
		if table == "" {
			continue
		}
		enrolled = append(enrolled, ModelInfo{
			Table:           table,
			TenantColumn:    "tenant_id",
			PartitionColumn: "partition_id",
		})
	}
	return enrolled, nil
}

// looksTenanted reports whether m has tenancy-shaped fields without
// implementing Tenanted (strict enrollment detector).
func looksTenanted(m any) bool {
	t := reflect.TypeOf(m)
	if t == nil {
		return false
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	return structLooksTenanted(t)
}

func structLooksTenanted(t reflect.Type) bool {
	for i := range t.NumField() {
		if fieldLooksTenanted(t.Field(i)) {
			return true
		}
	}
	return false
}

func fieldLooksTenanted(f reflect.StructField) bool {
	if f.Anonymous {
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && (ft.Name() == "BaseModel" || structLooksTenanted(ft)) {
			return true
		}
	}
	if f.Name == "TenantID" || f.Name == "PartitionID" {
		return true
	}
	tag := f.Tag.Get("gorm")
	return tagHasColumn(tag, "tenant_id") || tagHasColumn(tag, "partition_id")
}

func tagHasColumn(tag, col string) bool {
	// gorm tags: "column:tenant_id;..." or contain tenant_id as column name
	parts := strings.Split(tag, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.EqualFold(p, "column:"+col) {
			return true
		}
		if strings.HasPrefix(strings.ToLower(p), "column:") &&
			strings.EqualFold(strings.TrimPrefix(strings.ToLower(p), "column:"), col) {
			return true
		}
	}
	return false
}

// tableNameFor resolves the SQL table name GORM uses for the supplied
// model, honouring TableName() methods, gorm struct tags, and the db's
// configured naming strategy — exactly the same resolution path GORM
// uses internally when running queries or AutoMigrate against the model.
func tableNameFor(db *gorm.DB, m any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", fmt.Errorf("tenancy: parse model: %w", err)
	}
	return stmt.Table, nil
}

// asPointer returns m unchanged when it is already a pointer (or not a
// struct); for struct values it returns a pointer to an addressable
// copy so pointer-receiver interface checks see the full method set.
func asPointer(m any) any {
	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Struct {
		return m
	}
	p := reflect.New(v.Type())
	p.Elem().Set(v)
	return p.Interface()
}
