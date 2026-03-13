package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client/oauth2/auth"
	"github.com/pitabwire/frame/config"
)

func TestRemoteSignerTokenSource(t *testing.T) {
	// Signing endpoint
	signerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
			return
		}
		assert.Equal(t, "my-client", req["client_id"])
		assert.NotEmpty(t, req["token_endpoint"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"client_assertion":      "signed.jwt.assertion",
			"client_assertion_type": "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
			"algorithm":             "RS256",
		})
	}))
	defer signerServer.Close()

	// Token endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, r.ParseForm()) {
			return
		}
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
		assert.Equal(t, "my-client", r.PostForm.Get("client_id"))
		assert.Equal(t, "signed.jwt.assertion", r.PostForm.Get("client_assertion"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-remote","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	ts, err := auth.NewRemoteSignerTokenSource(
		context.Background(),
		tokenServer.Client(),
		&remoteSignerStubCfg{
			clientID:  "my-client",
			signerURL: signerServer.URL,
		},
		tokenServer.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-remote", tok.AccessToken)
	assert.Equal(t, "Bearer", tok.TokenType)
}

func TestRemoteSignerWithAPIKey(t *testing.T) {
	const apiKey = "test-signer-api-key"

	signerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorised"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"client_assertion":      "signed.jwt.assertion",
			"client_assertion_type": "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
		})
	}))
	defer signerServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-apikey","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	ts, err := auth.NewRemoteSignerTokenSource(
		context.Background(),
		tokenServer.Client(),
		&remoteSignerStubCfg{
			clientID:     "my-client",
			signerURL:    signerServer.URL,
			signerAPIKey: apiKey,
		},
		tokenServer.URL,
	)
	require.NoError(t, err)

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "tok-apikey", tok.AccessToken)
}

func TestRemoteSignerSignerError(t *testing.T) {
	signerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"failed to fetch signing keys"}`))
	}))
	defer signerServer.Close()

	ts, err := auth.NewRemoteSignerTokenSource(
		context.Background(),
		signerServer.Client(),
		&remoteSignerStubCfg{
			clientID:  "my-client",
			signerURL: signerServer.URL,
		},
		"https://token.example.com",
	)
	require.NoError(t, err)

	_, err = ts.Token()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signer endpoint returned status 502")
}

type remoteSignerStubCfg struct {
	clientID     string
	signerURL    string
	signerAPIKey string
}

func (c *remoteSignerStubCfg) LoadOauth2Config(context.Context) error   { return nil }
func (c *remoteSignerStubCfg) GetOauth2WellKnownOIDC() string           { return "" }
func (c *remoteSignerStubCfg) GetOauth2WellKnownJwk() string            { return "" }
func (c *remoteSignerStubCfg) GetOauth2WellKnownJwkData() string        { return "" }
func (c *remoteSignerStubCfg) GetOauth2Issuer() string                  { return "" }
func (c *remoteSignerStubCfg) GetOauth2AuthorizationEndpoint() string   { return "" }
func (c *remoteSignerStubCfg) GetOauth2RegistrationEndpoint() string    { return "" }
func (c *remoteSignerStubCfg) GetOauth2TokenEndpoint() string           { return "" }
func (c *remoteSignerStubCfg) GetOauth2UserInfoEndpoint() string        { return "" }
func (c *remoteSignerStubCfg) GetOauth2RevocationEndpoint() string      { return "" }
func (c *remoteSignerStubCfg) GetOauth2EndSessionEndpoint() string      { return "" }
func (c *remoteSignerStubCfg) GetOauth2ServiceURI() string              { return "" }
func (c *remoteSignerStubCfg) GetOauth2ServiceClientID() string         { return c.clientID }
func (c *remoteSignerStubCfg) GetOauth2ServiceClientSecret() string     { return "" }
func (c *remoteSignerStubCfg) GetOauth2TokenEndpointAuthMethod() string { return "" }
func (c *remoteSignerStubCfg) GetOauth2ServiceAudience() []string       { return nil }
func (c *remoteSignerStubCfg) GetOauth2ServiceAdminURI() string         { return "" }

func (c *remoteSignerStubCfg) GetOauth2PrivateKeyJWTConfig() *config.PrivateKeyJWTConfig {
	return &config.PrivateKeyJWTConfig{
		Source:       "url",
		SignerURL:    c.signerURL,
		SignerAPIKey: c.signerAPIKey,
	}
}
