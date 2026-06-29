package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/v2/datastore/dialect/postgres"
)

func TestNormalizeDSNLibpqPassthrough(t *testing.T) {
	t.Parallel()

	in := "host=localhost port=5432 user=u password=p dbname=test"
	out, err := postgres.NormalizeDSN(in)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestNormalizeDSNURIConversion(t *testing.T) {
	t.Parallel()

	out, err := postgres.NormalizeDSN("postgres://u:p@localhost:5432/test?sslmode=disable")
	require.NoError(t, err)
	require.Contains(t, out, "host=localhost")
	require.Contains(t, out, "port=5432")
	require.Contains(t, out, "user=u")
	require.Contains(t, out, "password=p")
	require.Contains(t, out, "dbname=test")
	require.Contains(t, out, "sslmode=disable")
}

func TestNormalizeDSNDefaultPort(t *testing.T) {
	t.Parallel()

	out, err := postgres.NormalizeDSN("postgres://u:p@localhost/test")
	require.NoError(t, err)
	require.Contains(t, out, "port=5432")
}

func TestNormalizeDSNRejectsBadScheme(t *testing.T) {
	t.Parallel()

	_, err := postgres.NormalizeDSN("mysql://u:p@localhost/test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid scheme",
		"expected invalid scheme error, got: %v", err)
}

func TestNormalizeDSNAcceptsPostgresql(t *testing.T) {
	t.Parallel()

	out, err := postgres.NormalizeDSN("postgresql://u:p@localhost/test")
	require.NoError(t, err)
	require.Contains(t, out, "dbname=test")
}
