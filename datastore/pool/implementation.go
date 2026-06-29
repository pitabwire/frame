package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pitabwire/util"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	dialectpg "github.com/pitabwire/frame/v2/datastore/dialect/postgres"
	"github.com/pitabwire/frame/v2/datastore/migration"
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"
)

const migrationAdvisoryLockID int64 = 82548391244719

type pool struct {
	readIdx     uint64
	writeIdx    uint64
	mu          sync.RWMutex
	allReadDBs  []*gorm.DB
	allWriteDBs []*gorm.DB
	closers     []func() error // close functions for opened pgxpools

	shouldDoMigrations bool

	adapter  dialect.DialectAdapter
	provider tenancy.Provider // may be nil if WithTenancyProvider(nil) was used
}

// NewPool constructs a pool. Options may set the dialect adapter and
// tenancy provider; defaults are Postgres + Postgres-RLS. The provider
// is wired into the adapter immediately, so hooks attach to every
// subsequently-opened connection.
func NewPool(_ context.Context, opts ...Option) Pool {
	o := &Options{
		PreferSimpleProtocol:   true,
		SkipDefaultTransaction: true,
		InsertBatchSize:        1000, //nolint:mnd // default insert batch size
		PreparedStatements:     true,
	}
	for _, opt := range opts {
		opt(o)
	}

	adapter := o.DialectAdapter
	if adapter == nil {
		adapter = dialectpg.New()
	}

	var provider tenancy.Provider
	if o.TenancyProviderSet {
		provider = o.TenancyProvider
	} else {
		provider = tenpg.New()
	}

	if provider != nil {
		if err := provider.WireAdapter(adapter); err != nil {
			// WireAdapter only errors for a nil-hook contract violation
			// (concrete providers don't pass nil hooks). Panicking is
			// loud at boot for what is purely a programming bug.
			panic("pool: tenancy provider WireAdapter: " + err.Error())
		}
	}

	return &pool{
		allReadDBs:         []*gorm.DB{},
		allWriteDBs:        []*gorm.DB{},
		closers:            []func() error{},
		shouldDoMigrations: true,
		mu:                 sync.RWMutex{},
		adapter:            adapter,
		provider:           provider,
	}
}

// AddConnection opens a new physical connection through the adapter.
func (s *pool) AddConnection(ctx context.Context, opts ...Option) error {
	o := &Options{
		PreferSimpleProtocol:   true,
		SkipDefaultTransaction: true,
		InsertBatchSize:        1000, //nolint:mnd // default insert batch size
		PreparedStatements:     true,
	}
	for _, opt := range opts {
		opt(o)
	}

	for _, conn := range o.Connections {
		db, closeFn, err := s.createConnection(ctx, conn.DSN, o)
		if err != nil {
			return err
		}
		if s.provider != nil {
			if wireErr := s.provider.WireGorm(db); wireErr != nil {
				return wireErr
			}
		}

		s.mu.Lock()
		if conn.ReadOnly {
			s.allReadDBs = append(s.allReadDBs, db)
		} else {
			s.allWriteDBs = append(s.allWriteDBs, db)
		}
		s.closers = append(s.closers, closeFn)
		s.mu.Unlock()
	}
	return nil
}

// Close calls every per-connection close function (which closes both
// the *sql.DB and the underlying pgxpool, releasing all resources).
func (s *pool) Close(_ context.Context) {
	s.mu.RLock()
	closers := append([]func() error(nil), s.closers...)
	s.mu.RUnlock()

	for _, c := range closers {
		_ = c()
	}
}

// DB returns a tenancy-aware *gorm.DB. Tenancy is enforced at the
// connection-acquire level by the provider's hook — DB itself does
// not filter by tenant_id / partition_id.
func (s *pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var idx *uint64
	var selectedDB *gorm.DB
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
	if selectedDB == nil {
		return nil
	}
	return selectedDB.Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).
		WithContext(ctx)
}

func (s *pool) selectOne(p []*gorm.DB, idx *uint64) *gorm.DB {
	if len(p) == 0 {
		return nil
	}
	pos := atomic.AddUint64(idx, 1)
	i := (pos - 1) % uint64(len(p))
	return p[i]
}

func (s *pool) CanMigrate() bool { return s.shouldDoMigrations }

func (s *pool) SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error {
	executor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB { return s.DB(ctx, false) })
	for _, p := range migrationPatches {
		if err := executor.SaveMigrationString(ctx, p.Name, p.Patch, p.RevertPatch); err != nil {
			return err
		}
	}
	return nil
}

// Migrate runs AutoMigrate on the supplied models, invokes
// Provider.Install on the tenancy-enrolled subset, and applies any
// patch-based migrations from migrationsDirPath. Uses the adapter's
// advisory lock to serialise concurrent boots.
func (s *pool) Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error {
	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	db := s.DB(ctx, false)
	if db == nil {
		return errors.New("migrate datastore: no writable database configured")
	}

	migrator := db.Migrator()

	unlock, lockErr := s.adapter.AdvisoryLock(ctx, db, migrationAdvisoryLockID)
	if lockErr != nil {
		util.Log(ctx).WithError(lockErr).
			Warn("MigrateDatastore -- couldn't acquire advisory lock, continuing without lock")
	}
	if unlock != nil {
		defer unlock()
	}

	if err := s.ensureMigrationTable(migrator); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't create migration table")
		return err
	}

	if err := s.applyAutoMigrations(ctx, db, migrator, migrations); err != nil {
		return err
	}

	executor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB { return s.DB(ctx, false) })
	if err := executor.ScanMigrationFiles(ctx, migrationsDirPath); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- Error scanning for new migrations")
		return err
	}
	if err := executor.ApplyNewMigrations(ctx); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- Error applying migrations ")
		return err
	}
	return nil
}

func (s *pool) ensureMigrationTable(migrator gorm.Migrator) error {
	if migrator.HasTable(&migration.Migration{}) {
		return nil
	}
	err := migrator.CreateTable(&migration.Migration{})
	if err != nil && !s.adapter.IsRelationAlreadyExistsErr(err) {
		return err
	}
	return nil
}

// applyAutoMigrations runs GORM AutoMigrate for the supplied models and, if a
// tenancy provider is configured, installs tenancy artefacts (RLS policies,
// etc.) on the models that opted in.
func (s *pool) applyAutoMigrations(
	ctx context.Context,
	db *gorm.DB,
	migrator gorm.Migrator,
	migrations []any,
) error {
	if len(migrations) == 0 {
		return nil
	}
	if err := migrator.AutoMigrate(migrations...); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't auto migrate")
		return err
	}
	if s.provider == nil {
		return nil
	}
	enrolled, err := tenancy.EnrolledModels(db, migrations)
	if err != nil {
		return err
	}
	if err = s.provider.Install(ctx, db, enrolled); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- tenancy install failed")
		return err
	}
	return nil
}
