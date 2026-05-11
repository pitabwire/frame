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
	"github.com/pitabwire/frame/security"
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

// DB returns a *gorm.DB for the supplied context. When a request-scoped
// transaction has been bound to ctx via WithRequestTx, that transaction
// is returned so the entire request — repository code several layers
// deep — shares the tenancy-scoped tx with no manual plumbing. When no
// transaction is bound, the standard pool routing applies: read-only
// callers prefer replicas, writes go to the primary, both wrapped in
// the auto-applied TenancyPartition scope so even single-table GORM
// queries are tenancy-filtered.
func (s *pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
	if tx := TxFromContext(ctx); tx != nil {
		return tx
	}

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

// WithTenancy implements Pool. See the interface comment for the full
// contract. Implementation summary:
//
//  1. Acquire a *gorm.DB from the standard DB(ctx, readOnly) path.
//  2. Open a transaction so SET LOCAL applies to subsequent statements.
//  3. Publish app.tenant_id (single value) and app.partition_id (a
//     comma-separated list, since one principal may legitimately span
//     multiple partitions) from the auth claims via
//     set_config(..., true). The `true` third argument is "is_local" —
//     the setting reverts when the transaction commits / rolls back,
//     so connections returned to the pool carry no leaked state.
//  4. Hand the transaction to the caller. Naive SQL inside fn is then
//     filtered by the RLS policies that read those session variables.
//
// The partition list is consumed by the database policy as
// `partition_id = ANY(string_to_array(current_setting('app.partition_id'), ','))`.
// Single-partition callers continue to work: the list has one element
// and ANY behaves as equality.
func (s *pool) WithTenancy(
	ctx context.Context,
	readOnly bool,
	fn func(tx *gorm.DB) error,
) error {
	// If a request-scoped tx is already bound, reuse it — no nested tx,
	// no nested SET LOCAL (the outer middleware already set the vars).
	if tx := TxFromContext(ctx); tx != nil {
		return fn(tx)
	}
	return s.DB(ctx, readOnly).Transaction(func(tx *gorm.DB) error {
		return applyTenancySessionConfig(ctx, tx, fn)
	})
}

// WithRequestTx opens a tenancy-scoped transaction the same way as
// WithTenancy, binds it to a child context, and runs fn with the
// bound context. Inside fn the application code is fully tenancy-
// blind: pool.DB(child, _) returns the bound transaction so naive
// repository code is automatically routed through the tenancy
// session-variable set + Row-Level Security policies.
//
// The transaction commits when fn returns nil and rolls back when fn
// returns an error. Use as a server interceptor / middleware so every
// inbound request runs inside a single tenancy-scoped transaction.
func (s *pool) WithRequestTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	if TxFromContext(ctx) != nil {
		// Already inside a request-scoped tx — nest fn directly.
		return fn(ctx)
	}
	return s.DB(ctx, false).Transaction(func(tx *gorm.DB) error {
		return applyTenancySessionConfig(ctx, tx, func(boundTx *gorm.DB) error {
			return fn(ContextWithTx(ctx, boundTx))
		})
	})
}

// applyTenancySessionConfig populates app.tenant_id and app.partition_id
// from the auth claims and invokes fn. The partition setting is a
// comma-separated list so principals spanning multiple partitions are
// honoured by the database policy.
func applyTenancySessionConfig(
	ctx context.Context,
	tx *gorm.DB,
	fn func(tx *gorm.DB) error,
) error {
	claim := security.ClaimsFromContext(ctx)
	if claim != nil && !security.IsTenancyChecksOnClaimSkipped(ctx) {
		if err := tx.Exec(
			"SELECT set_config('app.tenant_id', ?, true)",
			claim.GetTenantID(),
		).Error; err != nil {
			return err
		}
		partitionList := joinPartitions(claim.GetPartitionIDs())
		if err := tx.Exec(
			"SELECT set_config('app.partition_id', ?, true)",
			partitionList,
		).Error; err != nil {
			return err
		}
	}
	return fn(tx)
}

// joinPartitions serialises a partition list to the comma-separated
// string format the RLS policy expects. Empty list serialises to "" so
// the policy's "setting is empty → match-all" branch kicks in (same as
// "no claims attached").
func joinPartitions(partitions []string) string {
	switch len(partitions) {
	case 0:
		return ""
	case 1:
		return partitions[0]
	default:
		// strings.Join would do; small inline equivalent avoids the import
		// in this file when joinPartitions is the sole consumer.
		total := len(partitions) - 1
		for _, p := range partitions {
			total += len(p)
		}
		out := make([]byte, 0, total)
		for i, p := range partitions {
			if i > 0 {
				out = append(out, ',')
			}
			out = append(out, p...)
		}
		return string(out)
	}
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

		// Auto-enable Row-Level Security on every tenancy-partitioned
		// table. Detection is structural: any migration whose backing
		// model embeds data.BaseModel carries tenant_id + partition_id
		// columns and is therefore a tenancy-scoped table. The policy
		// reads app.tenant_id + app.partition_id session variables set
		// by WithTenancy / WithRequestTx; together they enforce
		// isolation transparently for both GORM-builder and Raw-SQL
		// paths.
		if err = enableRowLevelSecurity(ctx, db, migrations); err != nil {
			util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't enable row level security")
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
