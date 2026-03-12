package client //nolint:testpackage // tests exercise package internals directly

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client/oauth2/signer"
	"github.com/pitabwire/frame/config"
)

const testClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

type oauth2AutoConfig struct {
	tokenEndpoint string
	clientID      string
	clientSecret  string
	authMethod    string
	privateJWT    *config.PrivateKeyJWTConfig
	audience      []string
}

func (c *oauth2AutoConfig) LoadOauth2Config(context.Context) error   { return nil }
func (c *oauth2AutoConfig) GetOauth2WellKnownOIDC() string           { return "" }
func (c *oauth2AutoConfig) GetOauth2WellKnownJwk() string            { return "" }
func (c *oauth2AutoConfig) GetOauth2WellKnownJwkData() string        { return "" }
func (c *oauth2AutoConfig) GetOauth2Issuer() string                  { return "" }
func (c *oauth2AutoConfig) GetOauth2AuthorizationEndpoint() string   { return "" }
func (c *oauth2AutoConfig) GetOauth2RegistrationEndpoint() string    { return "" }
func (c *oauth2AutoConfig) GetOauth2TokenEndpoint() string           { return c.tokenEndpoint }
func (c *oauth2AutoConfig) GetOauth2UserInfoEndpoint() string        { return "" }
func (c *oauth2AutoConfig) GetOauth2RevocationEndpoint() string      { return "" }
func (c *oauth2AutoConfig) GetOauth2EndSessionEndpoint() string      { return "" }
func (c *oauth2AutoConfig) GetOauth2ServiceURI() string              { return "" }
func (c *oauth2AutoConfig) GetOauth2ServiceClientID() string         { return c.clientID }
func (c *oauth2AutoConfig) GetOauth2ServiceClientSecret() string     { return c.clientSecret }
func (c *oauth2AutoConfig) GetOauth2TokenEndpointAuthMethod() string { return c.authMethod }
func (c *oauth2AutoConfig) GetOauth2PrivateKeyJWTConfig() *config.PrivateKeyJWTConfig {
	return c.privateJWT
}
func (c *oauth2AutoConfig) GetOauth2ServiceAudience() []string { return c.audience }
func (c *oauth2AutoConfig) GetOauth2ServiceAdminURI() string   { return "" }

func TestNewHTTPClientPrivateKeyJWT(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	restore := signer.SetFetchX509SVIDsForTest(
		func(context.Context, ...workloadapi.ClientOption) ([]*x509svid.SVID, error) {
			return []*x509svid.SVID{{
				ID:         spiffeid.RequireFromString("spiffe://example.org/ns/default/sa/service-auth"),
				Hint:       "internal",
				PrivateKey: privateKey,
			}}, nil
		},
	)
	t.Cleanup(restore)

	var tokenURL string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
		assert.Equal(t, testClientAssertionTypeJWTBearer, r.PostForm.Get("client_assertion_type"))
		assert.Equal(t, "frame-client", r.PostForm.Get("client_id"))
		assert.Equal(t, []string{"api://payments"}, r.PostForm["audience"])

		assertion := r.PostForm.Get("client_assertion")
		assert.NotEmpty(t, assertion)

		parsed, parseErr := jwt.ParseWithClaims(
			assertion,
			&jwt.RegisteredClaims{},
			func(token *jwt.Token) (any, error) {
				assert.Equal(t, jwt.SigningMethodRS256.Alg(), token.Method.Alg())
				assert.Equal(t, "kid-1", token.Header["kid"])
				return &privateKey.PublicKey, nil
			},
		)
		assert.NoError(t, parseErr)
		if !assert.NotNil(t, parsed) {
			http.Error(w, "missing parsed token", http.StatusInternalServerError)
			return
		}
		assert.True(t, parsed.Valid)

		claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
		assert.True(t, ok)
		if !ok {
			http.Error(w, "missing claims", http.StatusInternalServerError)
			return
		}
		assert.Equal(t, "frame-client", claims.Issuer)
		assert.Equal(t, "frame-client", claims.Subject)
		assert.Equal(t, []string{tokenURL}, []string(claims.Audience))
		assert.NotNil(t, claims.ExpiresAt)
		assert.NotEmpty(t, claims.ID)

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token-1",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}))
	}))
	defer tokenServer.Close()
	tokenURL = tokenServer.URL

	resourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer access-token-1", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer resourceServer.Close()

	ctx := config.ToContext(context.Background(), &oauth2AutoConfig{
		tokenEndpoint: tokenServer.URL,
		clientID:      "frame-client",
		authMethod:    config.TokenEndpointAuthMethodPrivateKeyJWT,
		privateJWT: &config.PrivateKeyJWTConfig{
			Source:   config.PrivateKeyJWTSourceWorkloadAPI,
			SPIFFEID: "spiffe://example.org/ns/default/sa/service-auth",
			Hint:     "internal",
			KeyID:    "kid-1",
		},
		audience: []string{"api://payments"},
	})

	httpClient, err := newHTTPClient(ctx)
	require.NoError(t, err)

	resp, err := httpClient.Get(resourceServer.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestNewHTTPClientUnsupportedSignerReturnsError(t *testing.T) {
	ctx := config.ToContext(context.Background(), &oauth2AutoConfig{
		tokenEndpoint: "https://issuer.example.org/oauth2/token",
		clientID:      "frame-client",
		authMethod:    config.TokenEndpointAuthMethodPrivateKeyJWT,
		privateJWT: &config.PrivateKeyJWTConfig{
			PrivateKeyPath: "/tmp/client.pem",
		},
	})

	_, err := newHTTPClient(ctx)
	require.Error(t, err)
}

func TestNewHTTPClientPrivateKeyJWTAudiences(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	restore := signer.SetFetchX509SVIDsForTest(
		func(context.Context, ...workloadapi.ClientOption) ([]*x509svid.SVID, error) {
			return []*x509svid.SVID{{
				ID:         spiffeid.RequireFromString("spiffe://example.org/ns/default/sa/service-auth"),
				PrivateKey: privateKey,
			}}, nil
		},
	)
	t.Cleanup(restore)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			assert.Equal(t, "Bearer abc", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusNoContent)
			return
		}

		assert.NoError(t, r.ParseForm())
		assert.Equal(t, url.Values{
			"audience":              {"api://a", "api://b"},
			"client_assertion":      {r.PostForm.Get("client_assertion")},
			"client_assertion_type": {testClientAssertionTypeJWTBearer},
			"client_id":             {"frame-client"},
			"grant_type":            {"client_credentials"},
		}, r.PostForm)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"abc","token_type":"Bearer","expires_in":60}`))
	}))
	defer tokenServer.Close()

	ctx := config.ToContext(context.Background(), &oauth2AutoConfig{
		tokenEndpoint: tokenServer.URL,
		clientID:      "frame-client",
		authMethod:    config.TokenEndpointAuthMethodPrivateKeyJWT,
		privateJWT: &config.PrivateKeyJWTConfig{
			Source: config.PrivateKeyJWTSourceWorkloadAPI,
		},
		audience: []string{"api://a", "api://b"},
	})

	httpClient, err := newHTTPClient(ctx)
	require.NoError(t, err)

	resp, err := httpClient.Get(tokenServer.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestNewHTTPClientNoTokenSourceWithoutAuthMethod(t *testing.T) {
	ctx := config.ToContext(context.Background(), &oauth2AutoConfig{
		tokenEndpoint: "https://issuer.example.org/oauth2/token",
		clientID:      "frame-client",
		authMethod:    "",
	})

	httpClient, err := newHTTPClient(ctx)
	require.NoError(t, err)

	// No oauth2 transport wrapping — transport should not inject Authorization.
	_, isOAuth2 := httpClient.Transport.(interface{ CancelRequest(*http.Request) })
	assert.False(t, isOAuth2)
}
