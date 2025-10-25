package pool

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pitabwire/util"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/migration"
	"github.com/pitabwire/frame/datastore/scopes"
)

type pool struct {
	readIdx     uint64       // atomic counter for round-robin
	writeIdx    uint64       // atomic counter for round-robin
	mu          sync.RWMutex // protects db slices
	allReadDBs  []*gorm.DB   // track all read DBs
	allWriteDBs []*gorm.DB   // track all write DBs

	shouldDoMigrations bool
}

func NewPool(_ context.Context) Pool {
	store := &pool{
		allReadDBs:  []*gorm.DB{},
		allWriteDBs: []*gorm.DB{},

		shouldDoMigrations: true,
		mu:                 sync.RWMutex{},
	}

	return store
}

// AddConnection safely adds a DB connection to the pool.
func (s *pool) AddConnection(ctx context.Context, opts ...Option) error {
	poolOpts := &Options{
		MaxOpen:                0,
		MaxIdle:                0,
		MaxLifetime:            0,
		PreferSimpleProtocol:   true,
		SkipDefaultTransaction: true,
		InsertBatchSize:        1000,
	}

	for _, opt := range opts {
		opt(poolOpts)
	}

	for _, conn := range poolOpts.Connections {
		db, err := s.createConnection(ctx, conn.DSN, poolOpts)
		if err != nil {
			return err
		}

		s.mu.Lock()

		if conn.ReadOnly {
			s.allReadDBs = append(s.allReadDBs, db)
		} else {
			s.allWriteDBs = append(s.allWriteDBs, db)
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *pool) Close(_ context.Context) {
	for _, db := range s.allReadDBs {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
	for _, db := range s.allWriteDBs {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

// DB Returns a random item from the slice, or an error if the slice is empty.
func (s *pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
	var idx *uint64

	s.mu.RLock()
	if readOnly {
		idx = &s.readIdx
		if len(s.allReadDBs) != 0 {
			// This check ensures we are able to use the write db if no more read dbs exist
			s.mu.RUnlock()
			return s.selectOne(s.allReadDBs, idx).Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).
				WithContext(ctx).
				Scopes(scopes.TenancyPartition(ctx))
		}
	}

	idx = &s.writeIdx

	s.mu.RUnlock()
	db := s.selectOne(s.allWriteDBs, idx)

	if db == nil {
		return nil
	}

	return db.Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).
		WithContext(ctx).
		Scopes(scopes.TenancyPartition(ctx))
}

// selectOne uses atomic round-robin for high concurrency.
func (s *pool) selectOne(pool []*gorm.DB, idx *uint64) *gorm.DB {
	if len(pool) == 0 {
		return nil
	}
	pos := atomic.AddUint64(idx, 1)
	return pool[int(pos-1)%len(pool)] //nolint:gosec // G115: index is result of (val % len), always < len and fits in int.
}

func (s *pool) CanMigrate() bool {
	return s.shouldDoMigrations
}

func (s *pool) SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error {
	migrationExecutor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB {
		return s.DB(ctx, false)
	})
	for _, migrationPatch := range migrationPatches {
		err := migrationExecutor.SaveMigrationString(
			ctx,
			migrationPatch.Name,
			migrationPatch.Patch,
			migrationPatch.RevertPatch,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *pool) Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error {
	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	migrtor := s.DB(ctx, false).Migrator()
	// Ensure the migration schema exists
	if !migrtor.HasTable(&migration.Migration{}) {
		err := migrtor.CreateTable(&migration.Migration{})
		if err != nil {
			util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't create migration table")
			return err
		}
	}

	if len(migrations) > 0 {
		// Migrate the schema
		err := migrtor.AutoMigrate(migrations...)
		if err != nil {
			util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't auto migrate")
			return err
		}
	}

	migrationExecutor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB {
		return s.DB(ctx, false)
	})

	err := migrationExecutor.ScanMigrationFiles(ctx, migrationsDirPath)
	if err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- Error scanning for new migrations")
		return err
	}

	err = migrationExecutor.ApplyNewMigrations(ctx)
	if err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- Error applying migrations ")
		return err
	}
	return nil
}
