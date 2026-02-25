package migration //nolint:testpackage // tests access package internals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSaveMigrationStringWithoutDB(t *testing.T) {
	t.Parallel()

	m := NewMigrator(context.Background(), func(context.Context) *gorm.DB { return nil })
	err := m.SaveMigrationString(context.Background(), "001_test.sql", "SELECT 1;", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no database configured")
}

func TestApplyNewMigrationsWithoutDB(t *testing.T) {
	t.Parallel()

	m := NewMigrator(context.Background(), func(context.Context) *gorm.DB { return nil })
	err := m.ApplyNewMigrations(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no database configured")
}
