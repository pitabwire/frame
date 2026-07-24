package push_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/queue/push"
	"github.com/stretchr/testify/require"
)

func TestParseCommaSeparatedList(t *testing.T) {
	t.Parallel()
	require.Nil(t, push.ParseCommaSeparatedList(""))
	require.Nil(t, push.ParseCommaSeparatedList("  "))
	require.Equal(t, []string{"a", "b"}, push.ParseCommaSeparatedList("a, b"))
	require.Equal(t, []string{"a@b.com"}, push.ParseCommaSeparatedList(" a@b.com , "))
}

func TestPrincipalAllowed(t *testing.T) {
	t.Parallel()
	// Exported via package-level behaviour through OIDCAuth; unit-test matcher
	// with a signed token below. Here exercise the empty-list and match paths
	// via Authenticate with a controlled JWKS.
	key, jwksURL, kid := mustTestRSAJWKS(t)

	cases := []struct {
		name    string
		allowed []string
		email   string
		sub     string
		wantErr error
	}{
		{
			name:    "no allowlist accepts any principal",
			allowed: nil,
			email:   "other@proj.iam.gserviceaccount.com",
			sub:     "12345",
			wantErr: nil,
		},
		{
			name:    "email match",
			allowed: []string{"tasks@proj.iam.gserviceaccount.com"},
			email:   "tasks@proj.iam.gserviceaccount.com",
			sub:     "999",
			wantErr: nil,
		},
		{
			name:    "email match case-insensitive",
			allowed: []string{"Tasks@Proj.iam.gserviceaccount.com"},
			email:   "tasks@proj.iam.gserviceaccount.com",
			sub:     "999",
			wantErr: nil,
		},
		{
			name:    "sub match when email empty",
			allowed: []string{"tasks@proj.iam.gserviceaccount.com"},
			email:   "",
			sub:     "tasks@proj.iam.gserviceaccount.com",
			wantErr: nil,
		},
		{
			name:    "not allowed",
			allowed: []string{"tasks@proj.iam.gserviceaccount.com"},
			email:   "evil@proj.iam.gserviceaccount.com",
			sub:     "evil",
			wantErr: queue.ErrForbidden,
		},
		{
			name:    "allowlist set but no principal claims",
			allowed: []string{"tasks@proj.iam.gserviceaccount.com"},
			email:   "",
			sub:     "",
			wantErr: queue.ErrForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			auth := push.NewOIDCAuth(t.Context(), push.OIDCConfig{
				Audience:      "https://svc.example/_frame/queue/orders",
				Issuers:       []string{"https://accounts.google.com"},
				JWKSURL:       jwksURL,
				JWKSRefresh:   time.Hour,
				AllowedEmails: tc.allowed,
			})
			t.Cleanup(auth.Stop)

			// Give JWKS a moment to load on first Start.
			require.Eventually(t, func() bool {
				token := mustSignOIDC(t, key, kid, map[string]any{
					"iss":   "https://accounts.google.com",
					"aud":   "https://svc.example/_frame/queue/orders",
					"email": tc.email,
					"sub":   tc.sub,
					"exp":   time.Now().Add(time.Hour).Unix(),
					"iat":   time.Now().Unix(),
				})
				req := httptest.NewRequest(http.MethodPost, "https://svc.example/_frame/queue/orders", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				err := auth.Authenticate(req)
				if tc.wantErr == nil {
					return err == nil
				}
				return err != nil && errors.Is(err, tc.wantErr)
			}, 3*time.Second, 20*time.Millisecond)
		})
	}
}

func TestOIDCAuth_RejectsBadAudienceAndIssuer(t *testing.T) {
	t.Parallel()
	key, jwksURL, kid := mustTestRSAJWKS(t)
	auth := push.NewOIDCAuth(t.Context(), push.OIDCConfig{
		Audience:    "https://svc.example/_frame/queue/orders",
		Issuers:     []string{"https://accounts.google.com"},
		JWKSURL:     jwksURL,
		JWKSRefresh: time.Hour,
	})
	t.Cleanup(auth.Stop)

	require.Eventually(t, func() bool {
		token := mustSignOIDC(t, key, kid, map[string]any{
			"iss": "https://accounts.google.com",
			"aud": "https://wrong.example/",
			"exp": time.Now().Add(time.Hour).Unix(),
			"sub": "x",
		})
		req := httptest.NewRequest(http.MethodPost, "https://svc.example/_frame/queue/orders", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return errors.Is(auth.Authenticate(req), queue.ErrForbidden)
	}, 3*time.Second, 20*time.Millisecond)

	token := mustSignOIDC(t, key, kid, map[string]any{
		"iss": "https://evil.example",
		"aud": "https://svc.example/_frame/queue/orders",
		"exp": time.Now().Add(time.Hour).Unix(),
		"sub": "x",
	})
	req := httptest.NewRequest(http.MethodPost, "https://svc.example/_frame/queue/orders", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	require.ErrorIs(t, auth.Authenticate(req), queue.ErrForbidden)
}

func TestOIDCAuth_MissingBearer(t *testing.T) {
	t.Parallel()
	auth := push.NewOIDCAuth(t.Context(), push.GoogleCloudTasksOIDCPreset("https://x"))
	t.Cleanup(auth.Stop)
	req := httptest.NewRequest(http.MethodPost, "https://x", nil)
	require.ErrorIs(t, auth.Authenticate(req), queue.ErrUnauthorized)
}

type testJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func mustTestRSAJWKS(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := "test-rsa"
	nBytes := key.PublicKey.N.Bytes()
	eBytes := big.NewInt(int64(key.PublicKey.E)).Bytes()
	body, err := json.Marshal(map[string]any{
		"keys": []testJWK{{
			Kty: "RSA",
			Kid: kid,
			Use: "sig",
			Alg: "RS256",
			N:   base64.RawURLEncoding.EncodeToString(nBytes),
			E:   base64.RawURLEncoding.EncodeToString(eBytes),
		}},
	})
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return key, srv.URL, kid
}

func mustSignOIDC(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	token.Header["kid"] = kid
	s, err := token.SignedString(key)
	require.NoError(t, err)
	return s
}
