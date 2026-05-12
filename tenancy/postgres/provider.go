package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pitabwire/frame/datastore/dialect"
	"github.com/pitabwire/frame/tenancy"
	"gorm.io/gorm"
)

// Provider is the Postgres concrete tenancy.Provider. It installs RLS
// policies during Migrate and binds per-request tenancy state via
// pgxpool acquire/release hooks (no transactions required).
type Provider struct{}

// New returns a fresh Postgres tenancy provider.
func New() *Provider { return &Provider{} }

// Name implements tenancy.Provider.
func (*Provider) Name() string { return "postgres-rls" }

// Capabilities implements tenancy.Provider.
func (*Provider) Capabilities() tenancy.Capabilities {
	return tenancy.Capabilities{EnforcesAtStorage: true}
}

// Install implements tenancy.Provider. Idempotent:
//   - CREATE OR REPLACE for the SQL function
//   - DROP POLICY IF EXISTS / CREATE POLICY pair per table
//   - ALTER TABLE … ENABLE / FORCE ROW LEVEL SECURITY (Postgres
//     no-ops if already enabled).
func (*Provider) Install(_ context.Context, db *gorm.DB, models []tenancy.ModelInfo) error {
	if db == nil {
		return errors.New("tenancy/postgres: nil db")
	}
	if err := db.Exec(appTenancyMatchesFn).Error; err != nil {
		return fmt.Errorf("install app_tenancy_matches: %w", err)
	}
	for _, m := range models {
		if applyErr := applyTenancyPolicy(db, m); applyErr != nil {
			return fmt.Errorf("enable RLS on %s: %w", m.Table, applyErr)
		}
	}
	return nil
}

// applyTenancyPolicy emits the four idempotent statements for one table.
func applyTenancyPolicy(db *gorm.DB, m tenancy.ModelInfo) error {
	quoted := pgQuoteIdent(m.Table)
	tenantCol := pgQuoteIdent(m.TenantColumn)
	partitionCol := pgQuoteIdent(m.PartitionColumn)

	stmts := []string{
		fmt.Sprintf(alterEnableRLS, quoted),
		fmt.Sprintf(alterForceRLS, quoted),
		fmt.Sprintf(dropPolicy, quoted),
		fmt.Sprintf(createPolicy, quoted, tenantCol, partitionCol, tenantCol, partitionCol),
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

// pgQuoteIdent double-quotes a Postgres identifier, escaping embedded quotes.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// WireAdapter implements tenancy.Provider. Registers a BeforeAcquire
// hook that pushes Claims-derived session vars onto the connection,
// and an AfterRelease hook that resets them so connections are returned
// to the pool clean.
func (p *Provider) WireAdapter(adapter dialect.DialectAdapter) error {
	if adapter == nil {
		return errors.New("tenancy/postgres: nil adapter")
	}
	if err := adapter.RegisterAcquireHook(p.beforeAcquire); err != nil {
		return fmt.Errorf("register acquire hook: %w", err)
	}
	if err := adapter.RegisterReleaseHook(p.afterRelease); err != nil {
		return fmt.Errorf("register release hook: %w", err)
	}
	return nil
}

// WireGorm implements tenancy.Provider. Postgres-RLS is enforced at
// the connection-acquire level, so no per-query GORM plugin is needed.
func (*Provider) WireGorm(_ *gorm.DB) error { return nil }

// beforeAcquire pulls the tenancy.Claims from ctx and pushes them onto
// the pgx connection as session variables. is_local=false means the
// vars persist for the conn's lifetime (not just one tx); afterRelease
// resets them. If claims are empty or Skip is set, no vars are pushed
// — the RLS policy's empty-match-all branch applies.
func (*Provider) beforeAcquire(ctx context.Context, conn dialect.DialectConn) error {
	claims := tenancy.ClaimsFromContext(ctx)
	if claims == nil || claims.IsEmpty() || claims.Skip {
		return nil
	}
	if err := conn.Exec(
		ctx,
		"SELECT set_config('app.tenant_id', $1, false)",
		claims.TenantID,
	); err != nil {
		return fmt.Errorf("set app.tenant_id: %w", err)
	}
	if err := conn.Exec(
		ctx,
		"SELECT set_config('app.partition_id', $1, false)",
		strings.Join(claims.PartitionIDs, ","),
	); err != nil {
		return fmt.Errorf("set app.partition_id: %w", err)
	}
	return nil
}

// afterRelease resets the session vars so subsequent acquires that
// don't carry tenancy claims see clean defaults.
func (*Provider) afterRelease(ctx context.Context, conn dialect.DialectConn) error {
	if err := conn.Exec(ctx, "RESET app.tenant_id"); err != nil {
		return err
	}
	return conn.Exec(ctx, "RESET app.partition_id")
}

var _ tenancy.Provider = (*Provider)(nil)
