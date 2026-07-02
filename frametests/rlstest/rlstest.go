// Copyright 2023-2026 Peter Bwire
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rlstest provides a test-only tenancy.Provider wrapper that
// drops connections to an unprivileged role so Postgres FORCE ROW
// LEVEL SECURITY is actually enforced.
//
// Background: the postgres testcontainer creates the application user
// as a SUPERUSER. Per Postgres semantics, superusers bypass RLS even
// when the policy is forced. Migrations need the elevated role (to
// create policies, force RLS, etc.), but application queries must run
// under a less-privileged role for the isolation guarantee to be
// exercised by tests. Promoted from service-fintech apps/limits/tests/rlstest so every
// frame service test suite can exercise RLS without hand-rolling the
// role-drop pattern (mirrors tenancy/postgres provider_test.go).
package rlstest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"

	// Pull in the pgx stdlib driver so database/sql can dial postgres
	// for role bootstrap and post-migration grants.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Role is the non-superuser role name used in tests.
const Role = "rls_test_user"

// Provider wraps the Postgres tenancy provider with hooks that switch
// the connection's role to Role on acquire and reset on release. The
// switch only fires after Enable has been called, so the migration
// phase runs as the original (superuser) role.
type Provider struct {
	inner   *tenpg.Provider
	enabled atomic.Bool
}

// New returns a fresh wrapper.
func New() *Provider {
	return &Provider{inner: tenpg.New()}
}

// Enable turns on the per-connection SET ROLE behaviour. Call after
// migration (and after grants on the migrated tables) and before
// issuing any application query you want to test against RLS.
func (p *Provider) Enable() { p.enabled.Store(true) }

// Name implements tenancy.Provider.
func (p *Provider) Name() string { return "rlstest+" + p.inner.Name() }

// Capabilities implements tenancy.Provider.
func (p *Provider) Capabilities() tenancy.Capabilities { return p.inner.Capabilities() }

// Install implements tenancy.Provider — delegates to the inner provider.
func (p *Provider) Install(ctx context.Context, db *gorm.DB, models []tenancy.ModelInfo) error {
	return p.inner.Install(ctx, db, models)
}

// WireAdapter implements tenancy.Provider. Registers our role-switch
// hooks AFTER the inner provider's tenancy hooks so the order on each
// acquire is: tenancy push (under superuser) → SET ROLE → query runs
// under the restricted role with the right session vars. On release:
// RESET ROLE → tenancy reset.
func (p *Provider) WireAdapter(adapter dialect.DialectAdapter) error {
	if adapter == nil {
		return errors.New("rlstest: nil adapter")
	}
	if err := p.inner.WireAdapter(adapter); err != nil {
		return err
	}
	if err := adapter.RegisterAcquireHook(p.beforeAcquire); err != nil {
		return fmt.Errorf("rlstest: register acquire hook: %w", err)
	}
	if err := adapter.RegisterReleaseHook(p.afterRelease); err != nil {
		return fmt.Errorf("rlstest: register release hook: %w", err)
	}
	return nil
}

// WireGorm implements tenancy.Provider — delegates to the inner provider.
func (p *Provider) WireGorm(db *gorm.DB) error { return p.inner.WireGorm(db) }

func (p *Provider) beforeAcquire(ctx context.Context, conn dialect.DialectConn) error {
	if !p.enabled.Load() {
		return nil
	}
	return conn.Exec(ctx, "SET ROLE "+Role)
}

func (p *Provider) afterRelease(ctx context.Context, conn dialect.DialectConn) error {
	if !p.enabled.Load() {
		return nil
	}
	return conn.Exec(ctx, "RESET ROLE")
}

// CreateRole opens a short-lived admin connection to dsn and creates
// the non-superuser Role idempotently. Safe to call before any tables
// exist — table-level grants must happen separately via GrantAll once
// the schema is migrated.
func CreateRole(ctx context.Context, dsn string) error {
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("rlstest: open admin db: %w", err)
	}
	defer adminDB.Close()
	if _, err = adminDB.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '`+Role+`') THEN
				CREATE ROLE `+Role+` NOLOGIN;
			END IF;
		END $$;
	`); err != nil {
		return fmt.Errorf("rlstest: create role: %w", err)
	}
	return nil
}

// GrantAll grants the test role enough privileges on the public schema
// to read and write every table that exists at call time. Must be
// invoked AFTER migration so all tables are present. Idempotent.
func GrantAll(ctx context.Context, dsn string) error {
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("rlstest: open admin db: %w", err)
	}
	defer adminDB.Close()

	stmts := []string{
		`GRANT USAGE ON SCHEMA public TO ` + Role,
		`GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA public TO ` + Role,
		`GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO ` + Role,
		// Default privileges so any table created later (e.g., by SQL
		// migration files run after AutoMigrate) is also accessible.
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public ` +
			`GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON TABLES TO ` + Role,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public ` +
			`GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO ` + Role,
	}
	for _, stmt := range stmts {
		if _, err = adminDB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("rlstest: %s: %w", stmt, err)
		}
	}
	return nil
}
