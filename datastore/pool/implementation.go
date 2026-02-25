package pool

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pitabwire/util"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/migration"
	"github.com/pitabwire/frame/datastore/scopes"
)

const migrationAdvisoryLockID int64 = 82548391244719
const migrationLockRetryInterval = 200 * time.Millisecond

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
		InsertBatchSize:        1000, //nolint:mnd // default insert batch size
		PreparedStatements:     true,
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
	s.mu.RLock()
	readDBs := append([]*gorm.DB(nil), s.allReadDBs...)
	writeDBs := append([]*gorm.DB(nil), s.allWriteDBs...)
	s.mu.RUnlock()

	for _, db := range readDBs {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
	for _, db := range writeDBs {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

// DB Returns a random item from the slice, or an error if the slice is empty.
func (s *pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
	var idx *uint64
	var selectedDB *gorm.DB

	s.mu.RLock()
	if readOnly {
		idx = &s.readIdx
		if len(s.allReadDBs) != 0 {
			selectedDB = s.selectOne(s.allReadDBs, idx)
		}
	}

	if selectedDB == nil {
		idx = &s.writeIdx
		selectedDB = s.selectOne(s.allWriteDBs, idx)
	}
	s.mu.RUnlock()

	if selectedDB == nil {
		return nil
	}

	return selectedDB.Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).
		WithContext(ctx).
		Scopes(scopes.TenancyPartition(ctx))
}

// selectOne uses atomic round-robin for high concurrency.
func (s *pool) selectOne(pool []*gorm.DB, idx *uint64) *gorm.DB {
	if len(pool) == 0 {
		return nil
	}
	pos := atomic.AddUint64(idx, 1)
	i := (pos - 1) % uint64(len(pool))
	return pool[i]
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

	db := s.DB(ctx, false)
	if db == nil {
		return errors.New("migrate datastore: no writable database configured")
	}

	migrtor := db.Migrator()

	unlock, lockErr := acquireMigrationLock(ctx, db)
	if lockErr != nil {
		util.Log(ctx).
			WithError(lockErr).
			Warn("MigrateDatastore -- couldn't acquire advisory lock, continuing without lock")
	}
	if unlock != nil {
		defer unlock()
	}

	// Ensure migration metadata table exists. Handle concurrent startup races gracefully.
	err := ensureMigrationTable(ctx, migrtor)
	if err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't create migration table")
		return err
	}

	if len(migrations) > 0 {
		// Migrate the schema
		err = migrtor.AutoMigrate(migrations...)
		if err != nil {
			util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't auto migrate")
			return err
		}
	}

	migrationExecutor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB {
		return s.DB(ctx, false)
	})

	err = migrationExecutor.ScanMigrationFiles(ctx, migrationsDirPath)
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

func ensureMigrationTable(_ context.Context, migrator gorm.Migrator) error {
	if migrator.HasTable(&migration.Migration{}) {
		return nil
	}

	err := migrator.CreateTable(&migration.Migration{})
	if err != nil && !isRelationAlreadyExistsErr(err) {
		return err
	}

	return nil
}

func acquireMigrationLock(ctx context.Context, db *gorm.DB) (func(), error) {
	if db == nil || db.Dialector.Name() != "postgres" {
		return nil, nil //nolint:nilnil // intentional: nil unlock func signals no lock was acquired
	}

	ticker := time.NewTicker(migrationLockRetryInterval)
	defer ticker.Stop()

	for {
		var acquired bool
		err := db.WithContext(ctx).
			Raw("SELECT pg_try_advisory_lock(?)", migrationAdvisoryLockID).
			Scan(&acquired).Error
		if err != nil {
			return nil, err
		}

		if acquired {
			return func() {
				unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = db.WithContext(unlockCtx).Exec("SELECT pg_advisory_unlock(?)", migrationAdvisoryLockID).Error
			}, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func isRelationAlreadyExistsErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P07"
	}

	return err != nil && strings.Contains(strings.ToLower(err.Error()), "already exists")
}
