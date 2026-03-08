package frame //nolint:testpackage // tests exercise package internals directly

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/config"
)

func TestOAuth2ClientJWKSRouteWithWorkloadAPI(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	restoreFetch := fetchOAuth2ClientJWKSSVIDs
	t.Cleanup(func() {
		fetchOAuth2ClientJWKSSVIDs = restoreFetch
	})

	fetchOAuth2ClientJWKSSVIDs = func(context.Context, ...workloadapi.ClientOption) ([]*x509svid.SVID, error) {
		return []*x509svid.SVID{
			{
				ID:         spiffeid.RequireFromString("spiffe://stawi.org/ns/auth/sa/other"),
				Hint:       "other",
				PrivateKey: privateKey,
			},
			{
				ID:         spiffeid.RequireFromString("spiffe://stawi.org/ns/profile/sa/service-profile"),
				Hint:       "internal",
				PrivateKey: privateKey,
			},
		}, nil
	}

	cfg := &config.ConfigurationDefault{
		Oauth2PrivateJwtKey: config.OAuth2PrivateJWTKeyConfig{
			Source:   config.PrivateKeyJWTSourceWorkloadAPI,
			SPIFFEID: "spiffe://stawi.org/ns/profile/sa/service-profile",
			Hint:     "internal",
			KeyID:    "service-profile",
		},
	}

	svc := &Service{configuration: cfg}
	mux := http.NewServeMux()
	svc.registerOAuth2ClientJWKSRoute(mux)

	req := httptest.NewRequest(http.MethodGet, defaultOAuth2ClientJWKSPath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var jwks jose.JSONWebKeySet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jwks))
	require.Len(t, jwks.Keys, 1)
	require.Equal(t, "service-profile", jwks.Keys[0].KeyID)
	require.Equal(t, "RS256", jwks.Keys[0].Algorithm)
}

func TestOAuth2ClientJWKSRouteSkippedWithoutWorkloadAPI(t *testing.T) {
	cfg := &config.ConfigurationDefault{
		Oauth2PrivateJwtKey: config.OAuth2PrivateJWTKeyConfig{
			PrivateKeyPath: "/tmp/service.key",
		},
	}

	svc := &Service{configuration: cfg}
	mux := http.NewServeMux()
	svc.registerOAuth2ClientJWKSRoute(mux)

	req := httptest.NewRequest(http.MethodGet, defaultOAuth2ClientJWKSPath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
