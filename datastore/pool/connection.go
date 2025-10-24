package pool

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func (s *pool) createConnection(ctx context.Context, dsn string, opts ...Option) (*gorm.DB, error) {
	poolOpts := &Options{
		MaxOpen:                0,
		MaxIdle:                0,
		MaxLifetime:            0,
		PreferSimpleProtocol:   true,
		SkipDefaultTransaction: true,
	}

	for _, opt := range opts {
		opt(poolOpts)
	}

	cleanedPostgresqlDSN, err := cleanPostgresDSN(dsn)
	if err != nil {
		return nil, err
	}

	cfg, err := pgxpool.ParseConfig(cleanedPostgresqlDSN)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	cfg.ConnConfig.Tracer = otelpgx.NewTracer()

	pgxPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	err = otelpgx.RecordStats(pgxPool)
	if err != nil {
		return nil, fmt.Errorf("unable to record database stats: %w", err)
	}

	conn := stdlib.OpenDBFromPool(pgxPool)

	gormDB, err := gorm.Open(
		postgres.New(postgres.Config{
			Conn:                 conn,
			PreferSimpleProtocol: poolOpts.PreferSimpleProtocol,
		}),
		&gorm.Config{
			Logger:                 datastoreLogger(ctx, poolOpts.TraceConfig),
			SkipDefaultTransaction: poolOpts.SkipDefaultTransaction,
		},
	)

	if err != nil {
		return nil, err
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
