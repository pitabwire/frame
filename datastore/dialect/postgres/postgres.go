package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/dialect"
)

const idleTimeToMaxLifeTimeDivisor = 2
const migrationLockRetryInterval = 200 * time.Millisecond
const releaseHookTimeout = 5 * time.Second

// Adapter is the Postgres concrete implementation of
// dialect.DialectAdapter. Construct with New; hook registration must
// happen BEFORE OpenConnection.
type Adapter struct {
	mu           sync.RWMutex
	acquireHooks []dialect.AcquireHook
	releaseHooks []dialect.ReleaseHook
}

// New returns a fresh Adapter with no registered hooks.
func New() *Adapter {
	return &Adapter{}
}

// Name implements dialect.DialectAdapter.
func (*Adapter) Name() string { return "postgres" }

// NormalizeDSN implements dialect.DialectAdapter.
func (*Adapter) NormalizeDSN(raw string) (string, error) {
	return NormalizeDSN(raw)
}

// QuoteIdentifier implements dialect.DialectAdapter.
func (*Adapter) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// IsRelationAlreadyExistsErr implements dialect.DialectAdapter.
// Detects SQLSTATE 42P07 ("relation already exists"), used to gracefully
// handle concurrent migration startup races.
func (*Adapter) IsRelationAlreadyExistsErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P07"
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// RegisterAcquireHook implements dialect.DialectAdapter.
func (a *Adapter) RegisterAcquireHook(fn dialect.AcquireHook) error {
	if fn == nil {
		return errors.New("dialect/postgres: nil acquire hook")
	}
	a.mu.Lock()
	a.acquireHooks = append(a.acquireHooks, fn)
	a.mu.Unlock()
	return nil
}

// RegisterReleaseHook implements dialect.DialectAdapter.
func (a *Adapter) RegisterReleaseHook(fn dialect.ReleaseHook) error {
	if fn == nil {
		return errors.New("dialect/postgres: nil release hook")
	}
	a.mu.Lock()
	a.releaseHooks = append(a.releaseHooks, fn)
	a.mu.Unlock()
	return nil
}

// snapshotHooks returns copies of the hook slices under a read lock
// so callbacks invoked by pgxpool observe a stable chain. Per the
// Adapter contract, RegisterX must run BEFORE OpenConnection; the
// RLock is therefore only protecting against concurrent registrations
// during the construction phase, not late-running register calls.
func (a *Adapter) snapshotHooks() ([]dialect.AcquireHook, []dialect.ReleaseHook) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	acq := append([]dialect.AcquireHook(nil), a.acquireHooks...)
	rel := append([]dialect.ReleaseHook(nil), a.releaseHooks...)
	return acq, rel
}

// pgxDialectConn wraps a *pgx.Conn (the underlying connection passed by
// pgxpool's PrepareConn / AfterRelease callbacks) so it satisfies
// dialect.DialectConn without leaking pgx types past the boundary.
type pgxDialectConn struct {
	c *pgx.Conn
}

func (w *pgxDialectConn) Exec(ctx context.Context, query string, args ...any) error {
	_, err := w.c.Exec(ctx, query, args...)
	return err
}

// runAcquireHooks is wired into pgxpool.PrepareConn. It invokes every
// registered acquire hook in registration order. pgxpool semantics of
// the (bool, error) return value:
//   - (true,  nil): conn accepted, used by caller.
//   - (true,  err): conn accepted, err returned to caller anyway.
//   - (false, nil): conn destroyed, transparent retry on a new conn.
//   - (false, err): conn destroyed, err returned to caller (no retry).
//
// Tenancy session-binding failures must surface to the caller — we
// therefore return (false, hookErr) on any hook error so the query
// fails fast rather than running on an unscoped conn.
func (a *Adapter) runAcquireHooks(ctx context.Context, conn *pgx.Conn) (bool, error) {
	acq, _ := a.snapshotHooks()
	if len(acq) == 0 {
		return true, nil
	}
	wrapped := &pgxDialectConn{c: conn}
	for _, h := range acq {
		if err := h(ctx, wrapped); err != nil {
			return false, err
		}
	}
	return true, nil
}

// runReleaseHooks dispatches all registered release hooks. pgxpool's
// AfterRelease has no ctx; use context.Background so reset SQL is not
// cancelled by an already-cancelled request ctx. Hooks must remain
// cheap. Returning false destroys the connection instead of returning
// it to the pool.
func (a *Adapter) runReleaseHooks(conn *pgx.Conn) bool {
	_, rel := a.snapshotHooks()
	if len(rel) == 0 {
		return true
	}
	wrapped := &pgxDialectConn{c: conn}
	// Use a bounded background ctx — pgxpool's AfterRelease has no ctx,
	// and a hung release hook would block the pool slot indefinitely.
	// 5s is generous for the only currently expected workload (RESET
	// session vars) but long enough that legitimate slow queries finish.
	hookCtx, cancel := context.WithTimeout(context.Background(), releaseHookTimeout)
	defer cancel()
	for _, h := range rel {
		if err := h(hookCtx, wrapped); err != nil {
			return false
		}
	}
	return true
}

// applyPoolSizing copies pool tuning from ConnectionOptions onto the
// pgxpool.Config. Pulled out of OpenConnection to keep that function
// below the cognitive-complexity threshold.
func applyPoolSizing(cfg *pgxpool.Config, opts dialect.ConnectionOptions) {
	if opts.MaxOpen > 0 {
		maxConns := opts.MaxOpen
		if maxConns > math.MaxInt32 {
			maxConns = math.MaxInt32
		}
		cfg.MaxConns = int32(maxConns)
	}
	if opts.MaxLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxLifetime
		cfg.MaxConnIdleTime = opts.MaxLifetime / idleTimeToMaxLifeTimeDivisor
	}
}

// configureSQLDB applies *sql.DB pool sizing. MaxIdleConns is forced
// to 0 so every release flows through pgxpool, which is the property
// the hook chain relies on for tenancy hook correctness.
func configureSQLDB(db *sql.DB, opts dialect.ConnectionOptions) {
	db.SetMaxIdleConns(0)
	if opts.MaxOpen > 0 {
		db.SetMaxOpenConns(opts.MaxOpen)
	}
	if opts.MaxLifetime > 0 {
		db.SetConnMaxLifetime(opts.MaxLifetime)
	}
}

// OpenConnection implements dialect.DialectAdapter.
//
// Pool sizing notes:
//   - MaxIdleConns is forced to 0 on the *sql.DB so every release goes
//     through pgxpool — this is the property the hook chain relies on
//     to guarantee PrepareConn fires per query, never leaking
//     session state between requests.
//   - MaxOpenConns mirrors pgxpool.MaxConns so sql.DB never tries to
//     open more conns than the pool allows.
func (a *Adapter) OpenConnection(
	ctx context.Context,
	dsn string,
	opts dialect.ConnectionOptions,
) (gorm.Dialector, *sql.DB, func() error, error) {
	cleanDSN, err := a.NormalizeDSN(dsn)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := pgxpool.ParseConfig(cleanDSN)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create connection pool: %w", err)
	}

	applyPoolSizing(cfg, opts)
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()

	// Wire PrepareConn / AfterRelease to dispatch through registered
	// hooks. Hooks see a stable snapshot — concurrent RegisterX after
	// construction won't disturb in-flight callbacks. PrepareConn
	// supersedes the deprecated BeforeAcquire and propagates hook
	// errors back to the acquirer.
	cfg.PrepareConn = a.runAcquireHooks
	cfg.AfterRelease = a.runReleaseHooks

	pgxPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	if statErr := otelpgx.RecordStats(pgxPool); statErr != nil {
		pgxPool.Close()
		return nil, nil, nil, fmt.Errorf("unable to record database stats: %w", statErr)
	}

	connector := stdlib.GetPoolConnector(pgxPool)
	sqlDB := sql.OpenDB(connector)
	configureSQLDB(sqlDB, opts)

	dialector := gormpostgres.New(gormpostgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: opts.PreferSimpleProtocol,
	})

	// closeFn fully tears down both layers. stdlib.GetPoolConnector
	// documents that closing the returned *sql.DB does NOT close the
	// underlying *pgxpool.Pool — the pool's goroutines and idle conns
	// stay alive (and any session-level state, e.g. advisory locks,
	// remains held). Close sqlDB first so no new acquires can be
	// issued via the connector, then close the pgxpool which is
	// synchronous and blocks until conns are released.
	closeFn := func() error {
		sqlDBErr := sqlDB.Close()
		pgxPool.Close()
		return sqlDBErr
	}

	return dialector, sqlDB, closeFn, nil
}

// AdvisoryLock implements dialect.DialectAdapter using Postgres
// pg_try_advisory_lock semantics. Pins a single *sql.Conn for the
// duration of the lock so that pg_try_advisory_lock (session-level)
// and pg_advisory_unlock run against the same Postgres session.
// Retries every migrationLockRetryInterval until ctx is cancelled or
// the lock is acquired.
func (a *Adapter) AdvisoryLock(ctx context.Context, db *gorm.DB, id int64) (func(), error) {
	if db == nil {
		return nil, errors.New("dialect/postgres: nil db")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("dialect/postgres: resolve sql.DB: %w", err)
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("dialect/postgres: pin connection: %w", err)
	}

	ticker := time.NewTicker(migrationLockRetryInterval)
	defer ticker.Stop()

	for {
		var acquired bool
		row := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", id)
		if scanErr := row.Scan(&acquired); scanErr != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("dialect/postgres: advisory lock %d: %w", id, scanErr)
		}
		if acquired {
			return func() {
				unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", id)
				// Return the pinned conn to the pool. Postgres releases
				// any session-level state when the conn is recycled.
				_ = conn.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ dialect.DialectAdapter = (*Adapter)(nil)
