package tenancy

import (
	"context"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
)

// Provider installs and enforces tenancy isolation at the storage
// layer. Implementations are database-specific; the bundled Postgres
// provider uses Row-Level Security policies, others might use views,
// query rewriting, or a different scheme entirely.
type Provider interface {
	// Name returns a short, stable identifier ("postgres-rls") used in
	// logs and diagnostics.
	Name() string

	// Capabilities advertises what the provider does so the pool can
	// decide whether a complementary fallback (e.g., GORM scope) is
	// required.
	Capabilities() Capabilities

	// Install applies storage-side enforcement schema (RLS policies,
	// views, etc.) for the supplied models. Called once per migration.
	// Implementations MUST be idempotent — Frame re-runs migration on
	// every boot.
	Install(ctx context.Context, db *gorm.DB, models []ModelInfo) error

	// WireAdapter registers dialect-level hooks. Called once when the
	// pool is constructed, BEFORE any connection is opened. Providers
	// that enforce per-acquire (Postgres-RLS) register here.
	WireAdapter(adapter dialect.DialectAdapter) error

	// WireGorm registers GORM-level callbacks on the supplied *gorm.DB.
	// Called once per opened connection. Providers that enforce
	// per-query (alternative dialects without per-acquire hooks)
	// register here. Postgres-RLS implements as a no-op.
	WireGorm(db *gorm.DB) error
}

// Capabilities describes the runtime behaviour of a Provider.
type Capabilities struct {
	// EnforcesAtStorage is true when the provider installs DB-side
	// rules that block access without per-query gating (e.g., RLS,
	// views). Used by the pool to skip any fallback scope it might
	// otherwise have applied.
	EnforcesAtStorage bool
}

// ModelInfo describes one tenancy-enrolled model for Install. Built by
// the tenancy package via reflective enrollment; providers don't
// reimplement detection.
//
// All fields are required. The conventional values are "tenant_id" /
// "partition_id"; the tenancy package's enrollment code populates them
// from those conventions, and providers MUST NOT assume defaults if a
// caller hand-builds a ModelInfo.
type ModelInfo struct {
	// Table is the SQL table name resolved through GORM's naming
	// strategy.
	Table string

	// TenantColumn is the SQL column carrying the tenant identifier
	// (conventionally "tenant_id").
	TenantColumn string

	// PartitionColumn is the SQL column carrying the partition
	// identifier (conventionally "partition_id").
	PartitionColumn string
}
