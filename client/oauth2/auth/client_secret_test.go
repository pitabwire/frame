package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client/oauth2/auth"
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

func TestNewBasicTokenSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "my-client", user)
		assert.Equal(t, "my-secret", pass)

		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
		assert.Empty(t, r.PostForm.Get("client_id"))
		assert.Empty(t, r.PostForm.Get("client_secret"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"basic-tok","token_type":"Bearer","expires_in":300}`))
	}))
	defer server.Close()

	ts, err := auth.NewBasicTokenSource(
		context.Background(),
		server.Client(),
		&stubOAuth2Config{
			clientID:     "my-client",
			clientSecret: "my-secret",
		},
		server.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "basic-tok", tok.AccessToken)
}

func TestNewPostTokenSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := r.BasicAuth()
		assert.False(t, ok)

		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
		assert.Equal(t, "my-client", r.PostForm.Get("client_id"))
		assert.Equal(t, "my-secret", r.PostForm.Get("client_secret"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"post-tok","token_type":"Bearer","expires_in":300}`))
	}))
	defer server.Close()

	ts, err := auth.NewPostTokenSource(
		context.Background(),
		server.Client(),
		&stubOAuth2Config{
			clientID:     "my-client",
			clientSecret: "my-secret",
		},
		server.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "post-tok", tok.AccessToken)
}

func TestNewBasicTokenSourceWithAudiences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, []string{"api://a", "api://b"}, r.PostForm["audience"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"aud-tok","token_type":"Bearer","expires_in":60}`))
	}))
	defer server.Close()

	ts, err := auth.NewBasicTokenSource(
		context.Background(),
		server.Client(),
		&stubOAuth2Config{
			clientID:     "my-client",
			clientSecret: "my-secret",
			audience:     []string{"api://a", "api://b"},
		},
		server.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "aud-tok", tok.AccessToken)
}

func TestNewBasicTokenSourceRejectsEmptyClientID(t *testing.T) {
	_, err := auth.NewBasicTokenSource(
		context.Background(),
		&http.Client{},
		&stubOAuth2Config{clientSecret: "s"},
		"http://token.example.com",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client ID")
}

func TestNewBasicTokenSourceRejectsEmptySecret(t *testing.T) {
	_, err := auth.NewBasicTokenSource(
		context.Background(),
		&http.Client{},
		&stubOAuth2Config{clientID: "c"},
		"http://token.example.com",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client secret")
}
