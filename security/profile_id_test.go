package security_test

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pitabwire/frame/v2/security"
	"github.com/stretchr/testify/require"
)

func TestGetProfileIDPrefersExplicitClaimOverJWTSub(t *testing.T) {
	t.Parallel()

	// Service-account shape: Hydra sub=client_id, profile_id in claims.
	claims := &security.AuthenticationClaims{
		ProfileID: "profile-bot-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-authentication",
		},
	}
	require.Equal(t, "profile-bot-1", claims.GetProfileID())

	// Nested under ext (Hydra default for non-top-level claims).
	claimsExt := &security.AuthenticationClaims{
		Ext: map[string]any{"profile_id": "profile-bot-2"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-profile",
		},
	}
	require.Equal(t, "profile-bot-2", claimsExt.GetProfileID())

	// User shape: sub is the profile.
	userClaims := &security.AuthenticationClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "profile-user-9",
		},
	}
	require.Equal(t, "profile-user-9", userClaims.GetProfileID())

	// Explicit claim wins over both ext and sub.
	priority := &security.AuthenticationClaims{
		ProfileID: "top-level",
		Ext:       map[string]any{"profile_id": "in-ext"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-client",
		},
	}
	require.Equal(t, "top-level", priority.GetProfileID())
}
