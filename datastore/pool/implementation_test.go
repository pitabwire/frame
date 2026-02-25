package pool

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestIsRelationAlreadyExistsErr(t *testing.T) {
	t.Parallel()

	require.True(t, isRelationAlreadyExistsErr(&pgconn.PgError{Code: "42P07"}))
	require.True(t, isRelationAlreadyExistsErr(errors.New("relation \"migrations\" already exists")))
	require.False(t, isRelationAlreadyExistsErr(&pgconn.PgError{Code: "23505"}))
	require.False(t, isRelationAlreadyExistsErr(nil))
}

func TestMigrateWithoutWritableDBReturnsError(t *testing.T) {
	t.Parallel()

	dbPool := NewPool(context.Background())
	err := dbPool.Migrate(context.Background(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no writable database configured")
}
