package tenancy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/tenancy"
)

type valueWidget struct {
	data.BaseModel
	Name string
}

func (valueWidget) TableName() string { return "value_widgets" }

// Models registered BY VALUE must still enroll: Tenanted has pointer
// receivers, and four services shipped without RLS because their
// migrate lists passed values (2026-06 audit).
func TestEnrolledModelsAcceptsValueRegistration(t *testing.T) {
	db := newMinimalDB(t)

	byValue, err := tenancy.EnrolledModels(db, []any{valueWidget{}})
	require.NoError(t, err)
	byPointer, err := tenancy.EnrolledModels(db, []any{&valueWidget{}})
	require.NoError(t, err)

	require.Len(t, byPointer, 1, "pointer registration is the baseline")
	require.Len(t, byValue, 1, "value registration must enroll identically")
	require.Equal(t, byPointer, byValue)
	require.Equal(t, "value_widgets", byValue[0].Table)
}
