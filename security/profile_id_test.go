package security_test

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pitabwire/frame/v2/security"
	"github.com/stretchr/testify/require"
)

func TestNormalizeIdentity_SubIsAlwaysProfileID(t *testing.T) {
	t.Parallel()

	// Machine token: Hydra wire sub=client_id, profile_id in claims.
	claims := &security.AuthenticationClaims{
		ProfileID: "profile-bot-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-authentication",
		},
	}
	claims.NormalizeIdentity()
	require.Equal(t, "profile-bot-1", claims.Subject, "sub must become profile_id")
	require.Equal(t, "profile-bot-1", claims.GetProfileID())
	sub, err := claims.GetSubject()
	require.NoError(t, err)
	require.Equal(t, "profile-bot-1", sub)

	// Nested under ext.
	claimsExt := &security.AuthenticationClaims{
		Ext: map[string]any{"profile_id": "profile-bot-2"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-profile",
		},
	}
	claimsExt.NormalizeIdentity()
	require.Equal(t, "profile-bot-2", claimsExt.Subject)
	require.Equal(t, "profile-bot-2", claimsExt.GetProfileID())

	// User token: sub already is profile_id.
	user := &security.AuthenticationClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "profile-user-9",
		},
	}
	user.NormalizeIdentity()
	require.Equal(t, "profile-user-9", user.Subject)
	require.Equal(t, "profile-user-9", user.ProfileID)
	require.Equal(t, "profile-user-9", user.GetProfileID())
}

func TestGetProfileIDPrefersExplicitClaimOverJWTSub(t *testing.T) {
	t.Parallel()

	claims := &security.AuthenticationClaims{
		ProfileID: "profile-bot-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-authentication",
		},
	}
	require.Equal(t, "profile-bot-1", claims.GetProfileID())

	claimsExt := &security.AuthenticationClaims{
		Ext: map[string]any{"profile_id": "profile-bot-2"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "service-profile",
		},
	}
	require.Equal(t, "profile-bot-2", claimsExt.GetProfileID())

	userClaims := &security.AuthenticationClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "profile-user-9",
		},
	}
	require.Equal(t, "profile-user-9", userClaims.GetProfileID())
}
