// Package postgres provides the Postgres concrete tenancy.Provider.
// It installs Row-Level Security policies at migration time and binds
// per-request tenancy state via pgxpool BeforeAcquire / AfterRelease
// hooks. Combined, application code never references tenant_id or
// partition_id directly.
package postgres

// appTenancyMatchesFn is the Postgres-side helper installed once per
// database. It reads the app.tenant_id and app.partition_id session
// variables set by the per-acquire hook and answers whether a row is
// visible to the current principal.
//
//   - tenant_id is single-valued: row matches when the row's tenant
//     equals the setting, or the setting is empty (system services /
//     migrations are unscoped — same default as the no-claims path).
//   - partition_id is a comma-separated list: row matches when the
//     row's partition appears in the list, or the setting is empty.
//     Principals that legitimately span multiple partitions thus see
//     rows from every partition they belong to without any application
//     code awareness.
//
//nolint:unused // used by Provider in Task 11
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

// alterEnableRLS, alterForceRLS, dropPolicy, createPolicy are the per-
// table statements applied by Install. Quoting is performed by the
// caller (via the dialect adapter's QuoteIdentifier or the in-file
// pgQuoteIdent helper) — these strings receive pre-quoted table and
// column names.
//
//nolint:unused // used by Provider in Task 11
const (
	alterEnableRLS = "ALTER TABLE %s ENABLE ROW LEVEL SECURITY"
	alterForceRLS  = "ALTER TABLE %s FORCE ROW LEVEL SECURITY"
	dropPolicy     = "DROP POLICY IF EXISTS app_tenancy_isolation ON %s"
	createPolicy   = "CREATE POLICY app_tenancy_isolation ON %s FOR ALL " +
		"USING (app_tenancy_matches(%s, %s)) " +
		"WITH CHECK (app_tenancy_matches(%s, %s))"
)
