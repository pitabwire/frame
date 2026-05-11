package pool

import (
	"context"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/migration"
)

// txContextKey is the context key under which a per-request transaction
// is stored by WithRequestTx. Pool.DB looks it up first so that any code
// receiving the request's context — including code several call layers
// deep — gets the transaction-bound *gorm.DB transparently, without
// having to know that tenancy is being enforced.
type txContextKey struct{}

// ContextWithTx returns a context with the supplied *gorm.DB bound as
// the request transaction. Public so tests and custom middleware can
// drive the binding directly; the normal path is WithRequestTx.
func ContextWithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

// TxFromContext returns the request-scoped transaction bound by
// WithRequestTx, or nil if none is bound. Pool.DB honours the binding
// automatically; this accessor is exposed for code that needs to
// observe whether a request is already inside a tenancy transaction.
func TxFromContext(ctx context.Context) *gorm.DB {
	if v, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok {
		return v
	}
	return nil
}

// Connection represents a single database connection configuration.
type Connection struct {
	DSN      string
	ReadOnly bool
}

type Pool interface {
	DB(ctx context.Context, readOnly bool) *gorm.DB

	// WithTenancy runs fn inside a database transaction in which the
	// Postgres session variables app.tenant_id and app.partition_id have
	// been populated from the auth claims attached to ctx. Combined with
	// Row-Level Security policies that consult current_setting() on
	// every tenancy-scoped table, this means callers can write SQL that
	// makes no mention of tenant_id / partition_id — the database
	// enforces isolation transparently.
	//
	// When ctx carries no claims (system services, migrations) or the
	// tenancy-checks-skipped flag is set, the session variables are left
	// unset and the RLS policy's NULL-OK branch keeps the row visible.
	// Same behaviour as the auto-applied TenancyPartition scope, so the
	// contract is consistent between GORM query-builder paths and
	// Raw-SQL paths.
	//
	// Use this for any Raw SQL or multi-table-join report where you do
	// not want tenancy logic in the application. For trivially-scoped
	// GORM-builder paths (.Model(&X{}).Where(...).Find(...)), the
	// auto-applied TenancyPartition scope already filters correctly and
	// this helper is unnecessary.
	WithTenancy(ctx context.Context, readOnly bool, fn func(tx *gorm.DB) error) error

	AddConnection(ctx context.Context, opts ...Option) error

	CanMigrate() bool
	SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error
	// Migrate finds missing migrations and records them in the database.
	Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error

	Close(ctx context.Context)
}
