// Package dialect defines the driver abstraction frame uses to talk to
// a database. A DialectAdapter is responsible for DSN normalisation,
// connection construction (gorm.Dialector + *sql.DB), advisory locking
// for migrations, identifier quoting, and registering per-connection
// hooks that providers (e.g., tenancy.Provider) attach to inject
// per-request state on connection acquire / release.
package dialect

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DialectAdapter abstracts every database-specific concern in the
// frame datastore layer. Implementations are concrete per database
// (postgres, mysql, sqlite); the pool composes one adapter with one
// tenancy provider.
//
//nolint:revive // Name is intentionally explicit at the package boundary.
type DialectAdapter interface {
	// Name returns a short, stable identifier ("postgres") used in
	// logs.
	Name() string

	// NormalizeDSN converts URI or libpq form into the dialect's
	// expected DSN. Returns the normalized string and an error if the
	// input cannot be parsed.
	NormalizeDSN(raw string) (string, error)

	// OpenConnection constructs a gorm.Dialector + underlying *sql.DB
	// for the supplied DSN. The adapter is responsible for wiring
	// observability (e.g. otelpgx) and tuning (pool sizes, lifetimes).
	// Registered hooks (RegisterAcquireHook / RegisterReleaseHook) must
	// be attached to the connection that is opened.
	OpenConnection(
		ctx context.Context,
		dsn string,
		opts ConnectionOptions,
	) (gorm.Dialector, *sql.DB, error)

	// AdvisoryLock acquires a cooperative lock for serializing
	// migrations. Returns a release function (nil release means lock
	// acquisition is not supported by this dialect, in which case the
	// caller should warn and proceed).
	AdvisoryLock(ctx context.Context, db *gorm.DB, id int64) (release func(), err error)

	// IsRelationAlreadyExistsErr discriminates "table already exists"
	// errors that occur during concurrent migration startup races.
	IsRelationAlreadyExistsErr(err error) bool

	// QuoteIdentifier returns the supplied identifier quoted in the
	// dialect's native form. Used by providers when emitting DDL.
	QuoteIdentifier(name string) string

	// RegisterAcquireHook adds a callback invoked on every connection
	// acquire from the underlying pool. Hooks are called in
	// registration order. Returning an error rejects the acquire.
	//
	// Must be called BEFORE OpenConnection — hooks registered after
	// connection construction are not retroactively attached.
	RegisterAcquireHook(fn AcquireHook) error

	// RegisterReleaseHook adds a callback invoked on every connection
	// release. Hooks are called in registration order. Returning an
	// error causes the conn to be destroyed instead of returned to the
	// pool — useful for poisoned state detection.
	RegisterReleaseHook(fn ReleaseHook) error
}

// AcquireHook is invoked before a connection is handed out from the
// pool. Receives the context carried into the acquire call (which is
// the context the caller used on the topmost db.QueryContext) and a
// DialectConn wrapper around the native connection.
type AcquireHook func(ctx context.Context, conn DialectConn) error

// ReleaseHook is invoked before a connection is returned to the pool.
type ReleaseHook func(ctx context.Context, conn DialectConn) error

// DialectConn is the minimal surface a hook needs. Each adapter wraps
// its native connection (e.g. *pgx.Conn) behind this interface so no
// driver types leak past the hook boundary.
//
//nolint:revive // Name is intentionally explicit at the package boundary.
type DialectConn interface {
	// Exec runs a parameterised statement on the connection without
	// returning rows. Used by hooks to issue session-binding SQL.
	Exec(ctx context.Context, query string, args ...any) error
}

// ConnectionOptions tune the connection pool. Mirrors the previously
// per-dialect Options fields so callers see one consistent shape.
type ConnectionOptions struct {
	MaxOpen                int
	MaxLifetime            time.Duration
	PreferSimpleProtocol   bool
	SkipDefaultTransaction bool
	InsertBatchSize        int
	PreparedStatements     bool
	Logger                 gormlogger.Interface
}
