package postgres_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/datastore/dialect"
	"github.com/pitabwire/frame/datastore/dialect/postgres"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
)

type AdapterTestSuite struct {
	tests.BaseTestSuite
}

func TestAdapterSuite(t *testing.T) {
	suite.Run(t, &AdapterTestSuite{})
}

func (s *AdapterTestSuite) TestQuoteIdentifierEscapesEmbeddedQuotes() {
	a := postgres.New()
	s.Require().Equal(`"table"`, a.QuoteIdentifier("table"))
	s.Require().Equal(`"tab""le"`, a.QuoteIdentifier(`tab"le`))
}

func (s *AdapterTestSuite) TestIsRelationAlreadyExistsErrDetection() {
	a := postgres.New()
	s.Require().True(a.IsRelationAlreadyExistsErr(&pgconn.PgError{Code: "42P07"}))
	s.Require().True(a.IsRelationAlreadyExistsErr(errors.New(`relation "x" already exists`)))
	s.Require().False(a.IsRelationAlreadyExistsErr(&pgconn.PgError{Code: "23505"}))
	s.Require().False(a.IsRelationAlreadyExistsErr(nil))
}

func (s *AdapterTestSuite) TestOpenConnectionRunsAcquireHook() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		a := postgres.New()
		var acquired int32
		require.NoError(t, a.RegisterAcquireHook(func(_ context.Context, _ dialect.DialectConn) error {
			atomic.AddInt32(&acquired, 1)
			return nil
		}))

		_, sqlDB, closeFn, err := a.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 2})
		require.NoError(t, err)
		defer func() { _ = closeFn() }()

		row := sqlDB.QueryRowContext(ctx, "SELECT 1")
		var got int
		require.NoError(t, row.Scan(&got))
		require.Equal(t, 1, got)

		// At least one acquire happened to serve SELECT 1.
		require.GreaterOrEqual(t, atomic.LoadInt32(&acquired), int32(1))
	})
}

func (s *AdapterTestSuite) TestAcquireHookFailureDropsConn() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		a := postgres.New()
		require.NoError(t, a.RegisterAcquireHook(func(_ context.Context, _ dialect.DialectConn) error {
			return errors.New("boom")
		}))

		_, sqlDB, closeFn, err := a.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 1})
		require.NoError(t, err)
		defer func() { _ = closeFn() }()

		// With MaxOpen=1 and the hook returning an error, the acquire is
		// rejected and the query surfaces a database error.
		err = sqlDB.QueryRowContext(ctx, "SELECT 1").Scan(new(int))
		require.Error(t, err)
	})
}

func (s *AdapterTestSuite) TestAdvisoryLockAcquireAndRelease() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		a := postgres.New()
		dialector, _, closeFn, err := a.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 4})
		require.NoError(t, err)
		defer func() { _ = closeFn() }()

		db, err := gorm.Open(dialector, &gorm.Config{})
		require.NoError(t, err)

		release, err := a.AdvisoryLock(ctx, db, 999_888_777)
		require.NoError(t, err)
		require.NotNil(t, release)
		release()

		// Re-acquire after release succeeds.
		release2, err := a.AdvisoryLock(ctx, db, 999_888_777)
		require.NoError(t, err)
		release2()
	})
}
