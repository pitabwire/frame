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

	if len(poolOpts.DSNMap) > 0 {
		p := pool.NewPool(ctx)

		for dsn, readOnly := range poolOpts.DSNMap {
			err := p.AddConnection(ctx, dsn, readOnly, poolOpts.PoolOptions...)
			if err != nil {
				return nil, err
			}
		}
	}

	return man, nil
}

func (m *manager) AddPool(_ context.Context, reference string, store pool.Pool) {
	m.dbPools.Store(reference, store)
}

func (m *manager) RemovePool(ctx context.Context, reference string) {
	pl := m.GetPool(ctx, reference)
	if pl != nil {
		pl.Close(ctx)
	}
	m.dbPools.Delete(reference)
}

func (m *manager) GetPool(_ context.Context, reference string) pool.Pool {
	v, ok := m.dbPools.Load(reference)
	if !ok {
		return nil
	}

	pVal, ok := v.(pool.Pool)
	if !ok {
		return nil // Or log an error, depending on desired behavior
	}
	return pVal
}

// DB returns the database connection for the dbPool.
func (m *manager) DB(ctx context.Context, readOnly bool) *gorm.DB {
	return m.DBWithPool(ctx, datastore.DefaultPoolName, readOnly)
}

func (m *manager) DBWithPool(ctx context.Context, name string, readOnly bool) *gorm.DB {
	store := m.GetPool(ctx, name)
	if store == nil {
		return nil
	}
	return store.DB(ctx, readOnly)
}

func (m *manager) Close(ctx context.Context) {
	m.dbPools.Range(func(_, value interface{}) bool {
		pl, ok := value.(pool.Pool)
		if ok {
			pl.Close(ctx)
		}
		return true
	})
}

func (m *manager) SaveMigration(ctx context.Context, pool pool.Pool, migrationPatches ...*migration.Patch) error {
	return pool.SaveMigration(ctx, migrationPatches...)
}

// Migrate finds missing migrations and records them in the database.
func (m *manager) Migrate(ctx context.Context, pool pool.Pool, migrationsDirPath string, migrations ...any) error {
	return pool.Migrate(ctx, migrationsDirPath, migrations...)
}
