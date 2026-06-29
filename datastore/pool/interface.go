package pool

import (
	"context"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/migration"
)

// Connection represents a single database connection configuration.
type Connection struct {
	DSN      string
	ReadOnly bool
}

// Pool is the minimal connection-pool surface. Tenancy enforcement is
// applied transparently by the dialect adapter + tenancy provider
// composed via pool options; the Pool itself exposes only routing,
// migration, and lifecycle methods. Multi-statement atomicity is
// caller-driven via gorm's db.Transaction(fn).
type Pool interface {
	// DB returns a *gorm.DB routed to a writable (readOnly=false) or
	// read-only (readOnly=true) connection. The returned session has
	// tenancy applied at the connection level — callers do not need
	// to filter by tenant_id or partition_id explicitly.
	DB(ctx context.Context, readOnly bool) *gorm.DB

	// AddConnection opens a new physical connection and adds it to the
	// pool. May be called multiple times for read/write replication.
	AddConnection(ctx context.Context, opts ...Option) error

	// CanMigrate reports whether this pool was constructed in a mode
	// that permits running migrations.
	CanMigrate() bool

	// SaveMigration records the supplied patches in the migrations
	// metadata table without applying them.
	SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error

	// Migrate finds missing migrations, applies them, and installs
	// tenancy enforcement (via the configured tenancy.Provider) on
	// every enrolled model.
	Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error

	// Close gracefully shuts down all opened connections.
	Close(ctx context.Context)
}
