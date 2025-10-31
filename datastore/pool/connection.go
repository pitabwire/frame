package pool

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const idleTimeToMaxLifeTimeDivisor = 2

func (s *pool) createConnection(ctx context.Context, dsn string, poolOpts *Options) (*gorm.DB, error) {
	cleanedPostgresqlDSN, err := cleanPostgresDSN(dsn)
	if err != nil {
		return nil, err
	}

	cfg, err := pgxpool.ParseConfig(cleanedPostgresqlDSN)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// Configure pgxpool settings from Options
	if poolOpts.MaxOpen > 0 {
		cfg.MaxConns = int32(poolOpts.MaxOpen)
	}
	// Note: We intentionally don't set MinConns from MaxIdle here because:
	// 1. sql.DB will have MaxIdleConns set to 0 (required by GetPoolConnector)
	// 2. pgxpool will manage its own connection lifecycle
	// 3. Setting MinConns would keep connections open that sql.DB won't use
	if poolOpts.MaxLifetime > 0 {
		cfg.MaxConnLifetime = poolOpts.MaxLifetime
		cfg.MaxConnIdleTime = poolOpts.MaxLifetime / idleTimeToMaxLifeTimeDivisor // Set idle time to half of max lifetime
	}

	// Add OpenTelemetry tracing
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()

	// Create the pgxpool with configured settings
	pgxPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	// Record pool statistics for observability
	err = otelpgx.RecordStats(pgxPool)
	if err != nil {
		return nil, fmt.Errorf("unable to record database stats: %w", err)
	}

	// Use stdlib connector to expose pgxpool as database/sql compatible interface
	// This maintains the pool semantics while providing sql.DB interface for GORM
	connector := stdlib.GetPoolConnector(pgxPool)
	sqlDB := sql.OpenDB(connector)

	// CRITICAL: Set MaxIdleConns to 0 as per GetPoolConnector documentation
	// The pgxpool manages all connections internally. Setting idle connections on sql.DB
	// would cause it to hold connections outside the pool, starving direct pgxpool users.
	sqlDB.SetMaxIdleConns(0)

	// Set MaxOpenConns to match pgxpool's MaxConns to prevent sql.DB from trying
	// to open more connections than the pool allows
	if poolOpts.MaxOpen > 0 {
		sqlDB.SetMaxOpenConns(poolOpts.MaxOpen)
	}

	// Connection lifetime is managed by pgxpool, but we set it on sql.DB as well
	// to ensure consistency in connection recycling behavior
	if poolOpts.MaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(poolOpts.MaxLifetime)
	}

	// Open GORM with the properly configured sql.DB backed by pgxpool
	gormDB, err := gorm.Open(
		postgres.New(postgres.Config{
			Conn:                 sqlDB,
			PreferSimpleProtocol: poolOpts.PreferSimpleProtocol,
		}),
		&gorm.Config{
			Logger:                 datastoreLogger(ctx, poolOpts.TraceConfig),
			SkipDefaultTransaction: poolOpts.SkipDefaultTransaction,
            PrepareStmt:            poolOpts.PreparedStatements, // Controls prepared statement caching.
			CreateBatchSize:        poolOpts.InsertBatchSize,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to open GORM connection: %w", err)
	}

	return gormDB, nil
}

// cleanPostgresDSN checks if the input is already a DSN, otherwise converts a PostgreSQL URL to DSN.
func cleanPostgresDSN(pgString string) (string, error) {
	trimmed := strings.TrimSpace(pgString)
	// Heuristic: if it contains '=' and does not start with postgres:// or postgresql://, treat as DSN
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "=") && !strings.HasPrefix(lower, "postgres://") &&
		!strings.HasPrefix(lower, "postgresql://") {
		return trimmed, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	dbname := strings.TrimPrefix(u.Path, "/")

	dsn := []string{
		"host=" + host,
		"port=" + port,
		"user=" + user,
		"password=" + password,
		"dbname=" + dbname,
	}
	for k, vals := range u.Query() {
		for _, v := range vals {
			dsn = append(dsn, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(dsn, " "), nil
}
