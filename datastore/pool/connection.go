package pool

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/dialect"
)

// createConnection asks the adapter to open a *gorm.DB. The adapter
// owns all driver-specific concerns (DSN parsing, pgxpool tuning,
// otel wiring, hook attachment). Returns the gorm DB plus a close
// function the pool must invoke on shutdown.
func (s *pool) createConnection(ctx context.Context, dsn string, poolOpts *Options) (*gorm.DB, func() error, error) {
	cleanDSN, err := s.adapter.NormalizeDSN(dsn)
	if err != nil {
		return nil, nil, err
	}

	dialector, _, closeFn, err := s.adapter.OpenConnection(ctx, cleanDSN, dialect.ConnectionOptions{
		MaxOpen:                poolOpts.MaxOpen,
		MaxLifetime:            poolOpts.MaxLifetime,
		PreferSimpleProtocol:   poolOpts.PreferSimpleProtocol,
		SkipDefaultTransaction: poolOpts.SkipDefaultTransaction,
		InsertBatchSize:        poolOpts.InsertBatchSize,
		PreparedStatements:     poolOpts.PreparedStatements,
	})
	if err != nil {
		return nil, nil, err
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 datastoreLogger(ctx, poolOpts.TraceConfig),
		SkipDefaultTransaction: poolOpts.SkipDefaultTransaction,
		PrepareStmt:            poolOpts.PreparedStatements,
		CreateBatchSize:        poolOpts.InsertBatchSize,
	})
	if err != nil {
		_ = closeFn()
		return nil, nil, fmt.Errorf("failed to open GORM connection: %w", err)
	}
	return gormDB, closeFn, nil
}
