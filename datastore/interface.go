package datastore

import (
	"context"

	"github.com/pitabwire/frame/datastore/migration"
	"github.com/pitabwire/frame/datastore/pool"
)

const DefaultPoolName = "__default__pool_name__"

type Manager interface {
	AddPool(ctx context.Context, reference string, store pool.Pool)
	RemovePool(ctx context.Context, reference string)
	GetPool(ctx context.Context, reference string) pool.Pool
	Close(ctx context.Context)

	SaveMigration(ctx context.Context, pool pool.Pool, migrationPatches ...*migration.Patch) error
	// Migrate finds missing migrations and records them in the database.
	Migrate(ctx context.Context, pool pool.Pool, migrationsDirPath string, migrations ...any) error
}
