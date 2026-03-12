package oauth2_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oauth2source "github.com/pitabwire/frame/client/oauth2"
	"github.com/pitabwire/frame/client/oauth2/signer"
	"github.com/pitabwire/frame/config"
)

type stubOAuth2Config struct {
	tokenEndpoint string
	clientID      string
	clientSecret  string
	authMethod    string
	privateJWT    *config.PrivateKeyJWTConfig
	audience      []string
}

func (c *stubOAuth2Config) LoadOauth2Config(context.Context) error   { return nil }
func (c *stubOAuth2Config) GetOauth2WellKnownOIDC() string           { return "" }
func (c *stubOAuth2Config) GetOauth2WellKnownJwk() string            { return "" }
func (c *stubOAuth2Config) GetOauth2WellKnownJwkData() string        { return "" }
func (c *stubOAuth2Config) GetOauth2Issuer() string                  { return "" }
func (c *stubOAuth2Config) GetOauth2AuthorizationEndpoint() string   { return "" }
func (c *stubOAuth2Config) GetOauth2RegistrationEndpoint() string    { return "" }
func (c *stubOAuth2Config) GetOauth2TokenEndpoint() string           { return c.tokenEndpoint }
func (c *stubOAuth2Config) GetOauth2UserInfoEndpoint() string        { return "" }
func (c *stubOAuth2Config) GetOauth2RevocationEndpoint() string      { return "" }
func (c *stubOAuth2Config) GetOauth2EndSessionEndpoint() string      { return "" }
func (c *stubOAuth2Config) GetOauth2ServiceURI() string              { return "" }
func (c *stubOAuth2Config) GetOauth2ServiceClientID() string         { return c.clientID }
func (c *stubOAuth2Config) GetOauth2ServiceClientSecret() string     { return c.clientSecret }
func (c *stubOAuth2Config) GetOauth2TokenEndpointAuthMethod() string { return c.authMethod }
func (c *stubOAuth2Config) GetOauth2PrivateKeyJWTConfig() *config.PrivateKeyJWTConfig {
	return c.privateJWT
}
func (c *stubOAuth2Config) GetOauth2ServiceAudience() []string { return c.audience }
func (c *stubOAuth2Config) GetOauth2ServiceAdminURI() string   { return "" }

func TestNewTokenSourceClientSecretBasicExplicit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "my-client", user)
		assert.Equal(t, "my-secret", pass)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":60}`))
	}))
	defer server.Close()

	ts, err := oauth2source.NewTokenSource(
		context.Background(),
		server.Client(),
		&stubOAuth2Config{
			tokenEndpoint: server.URL,
			clientID:      "my-client",
			clientSecret:  "my-secret",
			authMethod:    "client_secret_basic",
		},
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok", tok.AccessToken)
}

func TestNewTokenSourceRejectsEmptyAuthMethod(t *testing.T) {
	_, err := oauth2source.NewTokenSource(
		context.Background(),
		&http.Client{},
		&stubOAuth2Config{
			tokenEndpoint: "http://tok",
			authMethod:    "",
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_endpoint_auth_method")
}

func TestNewTokenSourceClientSecretPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "my-client", r.PostForm.Get("client_id"))
		assert.Equal(t, "my-secret", r.PostForm.Get("client_secret"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-post","token_type":"Bearer","expires_in":60}`))
	}))
	defer server.Close()

	ts, err := oauth2source.NewTokenSource(
		context.Background(),
		server.Client(),
		&stubOAuth2Config{
			tokenEndpoint: server.URL,
			clientID:      "my-client",
			clientSecret:  "my-secret",
			authMethod:    "client_secret_post",
		},
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-post", tok.AccessToken)
}

func TestNewTokenSourcePrivateKeyJWT(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	restore := signer.SetFetchX509SVIDsForTest(
		func(context.Context, ...workloadapi.ClientOption) ([]*x509svid.SVID, error) {
			return []*x509svid.SVID{{
				ID:         spiffeid.RequireFromString("spiffe://example.org/svc"),
				PrivateKey: privateKey,
			}}, nil
		},
	)
	t.Cleanup(restore)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
		assert.NotEmpty(t, r.PostForm.Get("client_assertion"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-jwt",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	ts, err := oauth2source.NewTokenSource(
		context.Background(),
		server.Client(),
		&stubOAuth2Config{
			tokenEndpoint: server.URL,
			clientID:      "my-client",
			authMethod:    config.TokenEndpointAuthMethodPrivateKeyJWT,
			privateJWT: &config.PrivateKeyJWTConfig{
				Source: config.PrivateKeyJWTSourceWorkloadAPI,
			},
		},
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-jwt", tok.AccessToken)
}

func TestNewTokenSourceRejectsNilConfig(t *testing.T) {
	_, err := oauth2source.NewTokenSource(context.Background(), &http.Client{}, nil)
	require.Error(t, err)
}

func TestNewTokenSourceRejectsNilHTTPClient(t *testing.T) {
	_, err := oauth2source.NewTokenSource(
		context.Background(),
		nil,
		&stubOAuth2Config{tokenEndpoint: "http://tok"},
	)
	require.Error(t, err)
}

func TestNewTokenSourceRejectsUnsupportedMethod(t *testing.T) {
	_, err := oauth2source.NewTokenSource(
		context.Background(),
		&http.Client{},
		&stubOAuth2Config{
			tokenEndpoint: "http://tok",
			authMethod:    "magic_token",
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestNewTokenSourceRejectsEmptyTokenEndpoint(t *testing.T) {
	_, err := oauth2source.NewTokenSource(
		context.Background(),
		&http.Client{},
		&stubOAuth2Config{
			tokenEndpoint: "",
			clientID:      "c",
			clientSecret:  "s",
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token endpoint")
}
