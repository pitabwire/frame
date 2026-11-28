package postgres_test

import (
	"testing"

	"github.com/pitabwire/frame/v3/datastore/dialect/postgres"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDSN_ParseErrorNeverLeaksPassword(t *testing.T) {
	t.Parallel()
	const password = "normalize-pw-9"

	_, err := postgres.NormalizeDSN("postgres://app:" + password + "@db:bad port/app")

	require.Error(t, err)
	require.NotContains(t, err.Error(), password)
}
