package postgres_test

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/datastore/dialect/postgres"
	"github.com/stretchr/testify/require"
)

const testDSN = "host=localhost port=5432 user=u password=p dbname=d"

func TestQueryExecModeFollowsConnectionOptions(t *testing.T) {
	cases := []struct {
		name string
		opts dialect.ConnectionOptions
		want pgx.QueryExecMode
	}{
		{
			"simple protocol preferred",
			dialect.ConnectionOptions{PreferSimpleProtocol: true, PreparedStatements: true},
			pgx.QueryExecModeSimpleProtocol,
		},
		{"no prepared statements", dialect.ConnectionOptions{PreparedStatements: false}, pgx.QueryExecModeExec},
		{"prepared statements", dialect.ConnectionOptions{PreparedStatements: true}, pgx.QueryExecModeCacheStatement},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, postgres.QueryExecMode(testDSN, tc.opts))
		})
	}
}

func TestQueryExecModeDSNOverrideWins(t *testing.T) {
	dsn := testDSN + " default_query_exec_mode=cache_describe"
	got := postgres.QueryExecMode(dsn, dialect.ConnectionOptions{PreferSimpleProtocol: true, PreparedStatements: true})
	require.Equal(t, pgx.QueryExecModeCacheDescribe, got)
}
