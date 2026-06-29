package pool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/v2/datastore/pool"
)

func TestMigrateWithoutWritableDBReturnsError(t *testing.T) {
	t.Parallel()

	dbPool := pool.NewPool(context.Background())
	err := dbPool.Migrate(context.Background(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no writable database configured")
}
