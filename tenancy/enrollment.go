package tenancy

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// EnrolledModels filters the supplied migration models, returning
// ModelInfo for those that satisfy the Tenanted interface and do NOT
// satisfy Unscoped. Tenant and partition column names default to the
// conventional "tenant_id" / "partition_id"; future overrides can come
// from per-model tags but are not required today.
//
// The supplied *gorm.DB is currently unused; it is retained for a
// future extension to support per-model column name overrides via tags.
func EnrolledModels(_ *gorm.DB, models []any) ([]ModelInfo, error) {
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
		table := tableNameFor(m)
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

// tableNameFor resolves the SQL table name from the model type
// using GORM's default naming conventions (snake_case plural).
func tableNameFor(m any) string {
	t := reflect.TypeOf(m)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return ""
	}

	// Use GORM's naming strategy to convert type name to table name.
	// Default is snake_case plural (e.g. FakeTenanted -> fake_tenanteds).
	ns := schema.NamingStrategy{}
	return ns.TableName(t.Name())
}
