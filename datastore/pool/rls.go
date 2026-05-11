package pool

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/data"
)

// appTenancyMatchesFn is the Postgres-side helper installed once per
// database. It reads the app.tenant_id and app.partition_id session
// variables set by Pool.WithTenancy / Pool.WithRequestTx and answers
// whether a row is visible to the current principal.
//
//   - tenant_id is single-valued: row matches when the row's tenant
//     equals the setting, or the setting is empty (system services /
//     migrations are unscoped — same default as scopes.TenancyPartition
//     when no claims are present).
//   - partition_id is a comma-separated list: row matches when the
//     row's partition appears in the list, or the setting is empty.
//     Principals that legitimately span multiple partitions (an
//     operator with access to several branches, an analyst aggregating
//     across groups) thus see rows from every partition they belong
//     to without any application code awareness.
const appTenancyMatchesFn = `
CREATE OR REPLACE FUNCTION app_tenancy_matches(
    row_tenant_id text,
    row_partition_id text
) RETURNS boolean AS $$
BEGIN
    RETURN (
        current_setting('app.tenant_id', true) IS NULL
        OR current_setting('app.tenant_id', true) = ''
        OR row_tenant_id = current_setting('app.tenant_id', true)
    ) AND (
        current_setting('app.partition_id', true) IS NULL
        OR current_setting('app.partition_id', true) = ''
        OR row_partition_id = ANY(string_to_array(current_setting('app.partition_id', true), ','))
    );
END;
$$ LANGUAGE plpgsql STABLE;
`

// enableRowLevelSecurity installs the app_tenancy_matches function and
// applies an ALTER TABLE … ENABLE ROW LEVEL SECURITY + FORCE + a single
// FOR ALL policy on every tenancy-partitioned table represented in the
// supplied migration models. Detection is structural: any model that
// embeds data.BaseModel inherits tenant_id + partition_id columns and is
// considered tenancy-partitioned.
//
// The function is idempotent: re-running across a database that already
// has RLS enabled is a no-op (CREATE OR REPLACE FUNCTION + the DROP
// POLICY IF EXISTS / CREATE POLICY dance). Safe to call on every boot.
//
// Tables that don't embed BaseModel are left untouched — they aren't
// tenancy-partitioned in the first place (e.g. the migrations metadata
// table itself, or service-local lookup tables).
func enableRowLevelSecurity(_ context.Context, db *gorm.DB, migrations []any) error {
	if err := db.Exec(appTenancyMatchesFn).Error; err != nil {
		return fmt.Errorf("install app_tenancy_matches: %w", err)
	}

	for _, m := range migrations {
		if !embedsBaseModel(m) {
			continue
		}
		tableName, err := tableNameFor(db, m)
		if err != nil {
			return err
		}
		if tableName == "" {
			continue
		}
		if applyErr := applyTenancyPolicy(db, tableName); applyErr != nil {
			return fmt.Errorf("enable RLS on %s: %w", tableName, applyErr)
		}
	}
	return nil
}

// applyTenancyPolicy issues the per-table RLS setup. Separated so tests
// and admin tooling can target a known table by name without going
// through the migration scan.
func applyTenancyPolicy(db *gorm.DB, tableName string) error {
	quoted := pgQuoteIdent(tableName)
	stmts := []string{
		fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", quoted),
		fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY", quoted),
		fmt.Sprintf("DROP POLICY IF EXISTS app_tenancy_isolation ON %s", quoted),
		fmt.Sprintf(
			"CREATE POLICY app_tenancy_isolation ON %s FOR ALL "+
				"USING (app_tenancy_matches(tenant_id, partition_id)) "+
				"WITH CHECK (app_tenancy_matches(tenant_id, partition_id))",
			quoted,
		),
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

// embedsBaseModel reports whether the supplied migration model embeds
// data.BaseModel anywhere in its struct definition (direct or nested).
// We rely on reflection rather than type assertions so callers can pass
// in any GORM model without depending on a marker interface.
func embedsBaseModel(m any) bool {
	if m == nil {
		return false
	}
	t := reflect.TypeOf(m)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft == reflect.TypeOf(data.BaseModel{}) {
			return true
		}
		// Recurse into nested anonymous structs to support indirect
		// embedding (e.g. a domain base type that itself embeds
		// data.BaseModel).
		if ft.Kind() == reflect.Struct && embedsBaseModel(reflect.New(ft).Interface()) {
			return true
		}
	}
	return false
}

// tableNameFor resolves the SQL table name GORM uses for the supplied
// model. Uses GORM's statement parser so naming-strategy overrides
// (snake_case, plural, prefix, etc.) are respected without us
// hardcoding any conventions.
func tableNameFor(db *gorm.DB, m any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", fmt.Errorf("parse model: %w", err)
	}
	return stmt.Table, nil
}

// pgQuoteIdent double-quotes a Postgres identifier, escaping any
// embedded quote characters. Used to defend the ALTER / CREATE POLICY
// SQL against unusual table names without falling back to fmt.Sprintf
// of unsanitised input.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
