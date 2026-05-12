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

func TestEnrolledModelsPicksTenanted(t *testing.T) {
	t.Parallel()

	// tenancy.EnrolledModels uses GORM's statement parser, which works
	// without a real driver as long as we supply a *gorm.DB with a
	// naming strategy. Build a bare-bones DB just for table-name resolution.
	db := &gorm.DB{}

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
	db := &gorm.DB{}
	enrolled, err := tenancy.EnrolledModels(db, nil)
	require.NoError(t, err)
	require.Empty(t, enrolled)
}

func TestEnrolledModelsSkipsNil(t *testing.T) {
	t.Parallel()
	db := &gorm.DB{}
	enrolled, err := tenancy.EnrolledModels(db, []any{nil, &fakeTenanted{}, nil})
	require.NoError(t, err)
	require.Len(t, enrolled, 1)
}
