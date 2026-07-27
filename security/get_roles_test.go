package security_test

import (
	"testing"

	"github.com/pitabwire/frame/v2/security"
	"github.com/stretchr/testify/require"
)

func TestGetRolesFromExtArray(t *testing.T) {
	t.Parallel()

	claims := &security.AuthenticationClaims{
		Ext: map[string]any{
			// Hydra session extras + json.Unmarshal produce []any, not []string.
			"roles": []any{"internal"},
		},
	}
	require.Equal(t, []string{"internal"}, claims.GetRoles())
	require.True(t, claims.IsInternalSystem())
}

func TestGetRolesFromExtStringSlice(t *testing.T) {
	t.Parallel()

	claims := &security.AuthenticationClaims{
		Ext: map[string]any{
			"roles": []string{"admin", "internal"},
		},
	}
	require.Equal(t, []string{"admin", "internal"}, claims.GetRoles())
	require.True(t, claims.IsInternalSystem())
}

func TestGetRolesFromExtCommaString(t *testing.T) {
	t.Parallel()

	claims := &security.AuthenticationClaims{
		Ext: map[string]any{
			"roles": "admin, internal",
		},
	}
	require.Equal(t, []string{"admin", "internal"}, claims.GetRoles())
	require.True(t, claims.IsInternalSystem())
}

func TestGetRolesPrefersStructField(t *testing.T) {
	t.Parallel()

	claims := &security.AuthenticationClaims{
		Roles: []string{"member"},
		Ext: map[string]any{
			"roles": []any{"internal"},
		},
	}
	require.Equal(t, []string{"member"}, claims.GetRoles())
	require.False(t, claims.IsInternalSystem())
}
