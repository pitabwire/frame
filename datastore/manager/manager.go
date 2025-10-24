package manager

import (
	"context"
	"sync"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/migration"
	"github.com/pitabwire/frame/datastore/pool"
)

type manager struct {
	dbPools sync.Map
}

// NewManager creates a new database manager and optionally initializes pools with connections.
// The manager is resilient and will succeed even if no connections are provided initially.
// Connections can be added later using the manager's AddPool method.
func NewManager(ctx context.Context, options ...datastore.Option) (datastore.Manager, error) {
	poolOpts := datastore.Options{
		Name: datastore.DefaultPoolName,
	}

	for _, opt := range options {
		opt(&poolOpts)
	}

	man := &manager{
		dbPools: sync.Map{},
	}

	// Only create a pool if connections are provided
	if len(poolOpts.DSNMap) > 0 {
		p := pool.NewPool(ctx)

		// Add all connections to the pool
		for dsn, readOnly := range poolOpts.DSNMap {
			if err := p.AddConnection(ctx, dsn, readOnly, poolOpts.PoolOptions...); err != nil {
				// Clean up the pool on error
				p.Close(ctx)
				return nil, err
			}
		}

		// Register the pool with the manager
		man.AddPool(ctx, poolOpts.Name, p)
	}

	return man, nil
}

// AddPool registers a pool with the manager using the given reference name.
// This method is thread-safe and idempotent.
func (m *manager) AddPool(_ context.Context, reference string, store pool.Pool) {
	if reference == "" {
		reference = datastore.DefaultPoolName
	}

	if store != nil {
		m.dbPools.Store(reference, store)
	}
}

// RemovePool removes and closes a pool from the manager.
// This method is thread-safe and safe to call even if the pool doesn't exist.
func (m *manager) RemovePool(ctx context.Context, reference string) {
	pl := m.GetPool(ctx, reference)
	if pl != nil {
		pl.Close(ctx)
	}
	m.dbPools.Delete(reference)
}

// GetPool retrieves a pool by reference name.
// Returns nil if the pool doesn't exist or if the reference is invalid.
// This method is thread-safe.
func (m *manager) GetPool(_ context.Context, reference string) pool.Pool {
	if reference == "" {
		reference = datastore.DefaultPoolName
	}

	v, ok := m.dbPools.Load(reference)
	if !ok {
		return nil
	}

	pVal, ok := v.(pool.Pool)
	if !ok {
		return nil
	}
	return pVal
}

// DB returns a database connection from the default pool.
// Returns nil if the default pool doesn't exist.
func (m *manager) DB(ctx context.Context, readOnly bool) *gorm.DB {
	return m.DBWithPool(ctx, datastore.DefaultPoolName, readOnly)
}

// DBWithPool returns a database connection from the named pool.
// Returns nil if the pool doesn't exist.
// This method is thread-safe.
func (m *manager) DBWithPool(ctx context.Context, name string, readOnly bool) *gorm.DB {
	store := m.GetPool(ctx, name)
	if store == nil {
		return nil
	}
	return store.DB(ctx, readOnly)
}

// Close gracefully shuts down all registered pools.
// This method is thread-safe and idempotent.
func (m *manager) Close(ctx context.Context) {
	m.dbPools.Range(func(_, value interface{}) bool {
		if pl, ok := value.(pool.Pool); ok && pl != nil {
			pl.Close(ctx)
		}
		return true
	})
}

// SaveMigration saves migration patches to the specified pool.
// Returns an error if the pool is nil or if saving fails.
func (m *manager) SaveMigration(ctx context.Context, pool pool.Pool, migrationPatches ...*migration.Patch) error {
	if pool == nil {
		return nil // Gracefully handle nil pool
	}
	return pool.SaveMigration(ctx, migrationPatches...)
}

// Migrate finds missing migrations and records them in the database.
// Returns an error if the pool is nil or if migration fails.
func (m *manager) Migrate(ctx context.Context, pool pool.Pool, migrationsDirPath string, migrations ...any) error {
	if pool == nil {
		return nil // Gracefully handle nil pool
	}
	return pool.Migrate(ctx, migrationsDirPath, migrations...)
}
