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

func TestExchangeTokenViaBasicTokenSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "my-client", user)
		assert.Equal(t, "my-secret", pass)

		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-1","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	ts, err := auth.NewBasicTokenSource(
		context.Background(),
		server.Client(),
		&stubCfg{clientID: "my-client", clientSecret: "my-secret"},
		server.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-1", tok.AccessToken)
	assert.Equal(t, "Bearer", tok.TokenType)
	assert.False(t, tok.Expiry.IsZero())
}

func TestExchangeTokenOAuth2Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad credentials"}`))
	}))
	defer server.Close()

	ts, err := auth.NewBasicTokenSource(
		context.Background(),
		server.Client(),
		&stubCfg{clientID: "c", clientSecret: "s"},
		server.URL,
	)
	require.NoError(t, err)

	_, err = ts.Token()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_client")
	assert.Contains(t, err.Error(), "bad credentials")
}

func TestExchangeTokenMissingAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer"}`))
	}))
	defer server.Close()

	ts, err := auth.NewBasicTokenSource(
		context.Background(),
		server.Client(),
		&stubCfg{clientID: "c", clientSecret: "s"},
		server.URL,
	)
	require.NoError(t, err)

	_, err = ts.Token()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_token")
}

func TestExchangeTokenDefaultsBearerType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-2"}`))
	}))
	defer server.Close()

	ts, err := auth.NewBasicTokenSource(
		context.Background(),
		server.Client(),
		&stubCfg{clientID: "c", clientSecret: "s"},
		server.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "Bearer", tok.TokenType)
}

// stubCfg is a minimal ConfigurationOAUTH2 for testing.
type stubCfg struct {
	clientID     string
	clientSecret string
}

func (c *stubCfg) LoadOauth2Config(context.Context) error                    { return nil }
func (c *stubCfg) GetOauth2WellKnownOIDC() string                            { return "" }
func (c *stubCfg) GetOauth2WellKnownJwk() string                             { return "" }
func (c *stubCfg) GetOauth2WellKnownJwkData() string                         { return "" }
func (c *stubCfg) GetOauth2Issuer() string                                   { return "" }
func (c *stubCfg) GetOauth2AuthorizationEndpoint() string                    { return "" }
func (c *stubCfg) GetOauth2RegistrationEndpoint() string                     { return "" }
func (c *stubCfg) GetOauth2TokenEndpoint() string                            { return "" }
func (c *stubCfg) GetOauth2UserInfoEndpoint() string                         { return "" }
func (c *stubCfg) GetOauth2RevocationEndpoint() string                       { return "" }
func (c *stubCfg) GetOauth2EndSessionEndpoint() string                       { return "" }
func (c *stubCfg) GetOauth2ServiceURI() string                               { return "" }
func (c *stubCfg) GetOauth2ServiceClientID() string                          { return c.clientID }
func (c *stubCfg) GetOauth2ServiceClientSecret() string                      { return c.clientSecret }
func (c *stubCfg) GetOauth2TokenEndpointAuthMethod() string                  { return "" }
func (c *stubCfg) GetOauth2PrivateKeyJWTConfig() *config.PrivateKeyJWTConfig { return nil }
func (c *stubCfg) GetOauth2ServiceAudience() []string                        { return nil }
func (c *stubCfg) GetOauth2ServiceAdminURI() string                          { return "" }
