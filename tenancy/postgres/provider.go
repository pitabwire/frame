package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/tenancy"
)

// Provider is the Postgres concrete tenancy.Provider. It installs RLS
// policies during Migrate and binds per-request tenancy state via
// pgxpool acquire/release hooks (no transactions required).
//
// Security modes (see tenancy.SecurityMode):
//   - ModeFailOpen (default): nil/empty/Skip → clear session (match-all).
//   - ModeHybrid / ModeFailClosed: require bindable claims or authorized
//     SystemPrincipal; bare Skip rejected; partition-only rejected.
//
// Connection poolers: session-scoped GUCs require session affinity
// (direct Postgres or PgBouncer pool_mode=session). See docs/datastore.md.
type Provider struct {
	adapter             dialect.DialectAdapter
	mode                tenancy.SecurityMode
	allowGlobalServices map[string]struct{}
	enrollmentStrict    bool
}

// New returns a Postgres tenancy provider with optional configuration.
func New(opts ...Option) *Provider {
	p := &Provider{
		mode:                tenancy.ModeFailOpen,
		allowGlobalServices: make(map[string]struct{}),
	}
	for _, o := range opts {
		if o != nil {
			o(p)
		}
	}
	return p
}

// Name implements tenancy.Provider.
func (*Provider) Name() string { return "postgres-rls" }

// Capabilities implements tenancy.Provider.
func (*Provider) Capabilities() tenancy.Capabilities {
	return tenancy.Capabilities{EnforcesAtStorage: true}
}

// SetSecurityMode implements tenancy.ModeAware.
func (p *Provider) SetSecurityMode(m tenancy.SecurityMode) { p.mode = m }

// SecurityMode implements tenancy.ModeAware.
func (p *Provider) SecurityMode() tenancy.SecurityMode { return p.mode }

// SetAllowGlobalServices implements tenancy.AllowGlobalAware.
func (p *Provider) SetAllowGlobalServices(names ...string) {
	p.setAllowGlobalServices(names...)
}

// AllowGlobalServices implements tenancy.AllowGlobalAware.
func (p *Provider) AllowGlobalServices() []string {
	out := make([]string, 0, len(p.allowGlobalServices))
	for n := range p.allowGlobalServices {
		out = append(out, n)
	}
	return out
}

func (p *Provider) setAllowGlobalServices(names ...string) {
	if p.allowGlobalServices == nil {
		p.allowGlobalServices = make(map[string]struct{})
	}
	// Replace set when called from SetAllowGlobalServices / option.
	p.allowGlobalServices = make(map[string]struct{}, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			p.allowGlobalServices[n] = struct{}{}
		}
	}
}

// EnrollmentStrict reports whether strict enrollment is enabled.
func (p *Provider) EnrollmentStrict() bool { return p.enrollmentStrict }

// Install implements tenancy.Provider.
func (p *Provider) Install(_ context.Context, db *gorm.DB, models []tenancy.ModelInfo) error {
	if db == nil {
		return errors.New("tenancy/postgres: nil db")
	}
	if err := db.Exec(appTenancyMatchesFn).Error; err != nil {
		return fmt.Errorf("install app_tenancy_matches: %w", err)
	}
	for _, m := range models {
		if applyErr := p.applyTenancyPolicy(db, m); applyErr != nil {
			return fmt.Errorf("enable RLS on %s: %w", m.Table, applyErr)
		}
	}
	return nil
}

func (p *Provider) applyTenancyPolicy(db *gorm.DB, m tenancy.ModelInfo) error {
	quote := func(s string) string {
		if p.adapter != nil {
			return p.adapter.QuoteIdentifier(s)
		}
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	quoted := quote(m.Table)
	tenantCol := quote(m.TenantColumn)
	partitionCol := quote(m.PartitionColumn)

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

// WireAdapter implements tenancy.Provider.
func (p *Provider) WireAdapter(adapter dialect.DialectAdapter) error {
	if adapter == nil {
		return errors.New("tenancy/postgres: nil adapter")
	}
	p.adapter = adapter
	if err := adapter.RegisterAcquireHook(p.beforeAcquire); err != nil {
		return fmt.Errorf("register acquire hook: %w", err)
	}
	if err := adapter.RegisterReleaseHook(p.afterRelease); err != nil {
		return fmt.Errorf("register release hook: %w", err)
	}
	return nil
}

// WireGorm implements tenancy.Provider.
func (*Provider) WireGorm(_ *gorm.DB) error { return nil }

// beforeAcquire binds tenancy session GUCs per SecurityMode resolution order.
// See design: Transparent Multi-Tenant Isolation for Frame.
func (p *Provider) beforeAcquire(ctx context.Context, conn dialect.DialectConn) error {
	if sp, ok := tenancy.SystemPrincipalFromContext(ctx); ok {
		return p.acquireWithPrincipal(ctx, conn, sp)
	}
	return p.acquireWithClaims(ctx, conn, tenancy.ClaimsFromContext(ctx))
}

func (p *Provider) acquireWithPrincipal(
	ctx context.Context, conn dialect.DialectConn, sp tenancy.SystemPrincipal,
) error {
	// (1) Authorized global elevation → match-all
	if sp.AllowGlobal {
		if p.mode.IsSecure() && !p.allowGlobalAuthorized(ctx, sp) {
			return tenancy.ErrAllowGlobalDenied
		}
		return clearSession(ctx, conn)
	}
	// (2) Scoped system principal → bind principal scope
	if sp.TenantID == "" {
		return tenancy.ErrTenantIDRequired
	}
	bind := (&tenancy.Claims{TenantID: sp.TenantID, PartitionIDs: sp.PartitionIDs}).Normalize()
	if err := bind.Validate(); err != nil {
		return err
	}
	return bindSession(ctx, conn, bind)
}

func (p *Provider) acquireWithClaims(
	ctx context.Context, conn dialect.DialectConn, claims *tenancy.Claims,
) error {
	// (3) Explicit Skip without SystemPrincipal
	if claims != nil && claims.Skip {
		if p.mode == tenancy.ModeFailOpen {
			return clearSession(ctx, conn)
		}
		return tenancy.ErrSkipNotPermitted
	}
	// (4) Normal claims bind
	if claims != nil && !claims.IsEmpty() {
		claims = claims.Normalize()
		if p.mode.IsSecure() && claims.TenantID == "" {
			return tenancy.ErrTenantIDRequired
		}
		if err := claims.Validate(); err != nil {
			return err
		}
		return bindSession(ctx, conn, claims)
	}
	// (5) Missing / empty claims
	if p.mode == tenancy.ModeFailOpen {
		return clearSession(ctx, conn)
	}
	return tenancy.ErrClaimsRequired
}

func (p *Provider) allowGlobalAuthorized(ctx context.Context, sp tenancy.SystemPrincipal) bool {
	if tenancy.IsFrameworkMigration(ctx) {
		return true
	}
	if p.allowGlobalServices == nil {
		return false
	}
	_, ok := p.allowGlobalServices[sp.ServiceName]
	return ok
}

func bindSession(ctx context.Context, conn dialect.DialectConn, claims *tenancy.Claims) error {
	if err := conn.Exec(
		ctx,
		"SELECT set_config('app.tenant_id', $1, false), set_config('app.partition_id', $2, false)",
		claims.TenantID,
		strings.Join(claims.PartitionIDs, ","),
	); err != nil {
		return fmt.Errorf("set tenancy session vars: %w", err)
	}
	return nil
}

func (p *Provider) afterRelease(ctx context.Context, conn dialect.DialectConn) error {
	return clearSession(ctx, conn)
}

func clearSession(ctx context.Context, conn dialect.DialectConn) error {
	return conn.Exec(
		ctx,
		"SELECT set_config('app.tenant_id', '', false), set_config('app.partition_id', '', false)",
	)
}

var (
	_ tenancy.Provider         = (*Provider)(nil)
	_ tenancy.ModeAware        = (*Provider)(nil)
	_ tenancy.AllowGlobalAware = (*Provider)(nil)
)
