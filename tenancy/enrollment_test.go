package tenancy_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/tenancy"
)

// fakeTenanted satisfies tenancy.Tenanted minimally for the unit test.
type fakeTenanted struct {
	ID          string `gorm:"primaryKey"`
	TenantID    string
	PartitionID string
	AccessID    string
}

func (f *fakeTenanted) GetTenantID() string     { return f.TenantID }
func (f *fakeTenanted) GetPartitionID() string  { return f.PartitionID }
func (f *fakeTenanted) GetAccessID() string     { return f.AccessID }
func (f *fakeTenanted) SetTenantID(v string)    { f.TenantID = v }
func (f *fakeTenanted) SetPartitionID(v string) { f.PartitionID = v }
func (f *fakeTenanted) SetAccessID(v string)    { f.AccessID = v }

// fakeUntenanted intentionally lacks the tenant_id/partition_id fields.
type fakeUntenanted struct {
	ID   string `gorm:"primaryKey"`
	Name string
}

// fakeUnscoped embeds the marker to opt out even though it satisfies
// the Tenanted method set.
type fakeUnscoped struct {
	fakeTenanted
	tenancy.UnscopedMarker
}

// newMinimalDB creates a minimal but properly initialized gorm.DB
// suitable for statement parsing. No real database driver is used.
func newMinimalDB(t *testing.T) *gorm.DB {
	// gorm.Open with nil dialector still initializes config.cacheStore
	// and all required fields for statement parsing to work.
	db, err := gorm.Open(nil)
	require.NoError(t, err)
	return db
}

func TestEnrolledModelsPicksTenanted(t *testing.T) {
	t.Parallel()

	db := newMinimalDB(t)

	enrolled, err := tenancy.EnrolledModels(db, []any{
		&fakeTenanted{},
		&fakeUntenanted{},
		&fakeUnscoped{},
	})
	require.NoError(t, err)
	require.Len(t, enrolled, 1, "only fakeTenanted should be enrolled")
	require.Equal(t, "tenant_id", enrolled[0].TenantColumn)
	require.Equal(t, "partition_id", enrolled[0].PartitionColumn)
	require.NotEmpty(t, enrolled[0].Table)
}

func TestEnrolledModelsEmptyInput(t *testing.T) {
	t.Parallel()
	db := newMinimalDB(t)
	enrolled, err := tenancy.EnrolledModels(db, nil)
	require.NoError(t, err)
	require.Empty(t, enrolled)
}

func TestEnrolledModelsSkipsNil(t *testing.T) {
	t.Parallel()
	db := newMinimalDB(t)
	enrolled, err := tenancy.EnrolledModels(db, []any{nil, &fakeTenanted{}, nil})
	require.NoError(t, err)
	require.Len(t, enrolled, 1)
}

// modelWithTableNameOverride demonstrates a Tenanted model that overrides
// TableName() to use a non-default table name.
type modelWithTableNameOverride struct {
	ID          string `gorm:"primaryKey"`
	TenantID    string
	PartitionID string
	AccessID    string
}

func (m *modelWithTableNameOverride) GetTenantID() string     { return m.TenantID }
func (m *modelWithTableNameOverride) GetPartitionID() string  { return m.PartitionID }
func (m *modelWithTableNameOverride) GetAccessID() string     { return m.AccessID }
func (m *modelWithTableNameOverride) SetTenantID(v string)    { m.TenantID = v }
func (m *modelWithTableNameOverride) SetPartitionID(v string) { m.PartitionID = v }
func (m *modelWithTableNameOverride) SetAccessID(v string)    { m.AccessID = v }

// TableName overrides the default table name that GORM would infer.
func (m *modelWithTableNameOverride) TableName() string {
	return "weird_table"
}

func TestEnrolledModelsHonoursTableNameOverride(t *testing.T) {
	t.Parallel()
	db := newMinimalDB(t)

	enrolled, err := tenancy.EnrolledModels(db, []any{
		&modelWithTableNameOverride{},
	})
	require.NoError(t, err)
	require.Len(t, enrolled, 1)
	// Verify that the custom TableName() override is respected, not the
	// default snake_case plural derivation.
	require.Equal(t, "weird_table", enrolled[0].Table)
}
