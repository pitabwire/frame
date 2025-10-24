package pool

import (
	"context"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/migration"
)

// Connection represents a single database connection configuration.
type Connection struct {
	DSN      string
	ReadOnly bool
}

type Pool interface {
	DB(ctx context.Context, readOnly bool) *gorm.DB

	AddConnection(ctx context.Context, opts ...Option) error

	CanMigrate() bool
	SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error
	// Migrate finds missing migrations and records them in the database.
	Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error

	Close(ctx context.Context)
}
