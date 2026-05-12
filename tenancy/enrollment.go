package tenancy

import (
	"fmt"

	"gorm.io/gorm"
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
	if len(models) == 0 {
		return nil, nil
	}
	enrolled := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		if m == nil {
			continue
		}
		if _, isUnscoped := m.(Unscoped); isUnscoped {
			continue
		}
		if _, isTenanted := m.(Tenanted); !isTenanted {
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
