package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client/oauth2/auth"
)

func fixedNow() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}

func TestExchangeTokenSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-1","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	tok, err := auth.ExchangeToken(
		context.Background(),
		server.Client(),
		server.URL,
		auth.TokenEndpointRequest{
			Form: map[string][]string{
				"grant_type": {"client_credentials"},
			},
		},
		fixedNow,
	)
	require.NoError(t, err)
	assert.Equal(t, "tok-1", tok.AccessToken)
	assert.Equal(t, "Bearer", tok.TokenType)
	assert.Equal(t, fixedNow().Add(3600*time.Second), tok.Expiry)
}

func TestExchangeTokenWithBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "client-id", user)
		assert.Equal(t, "client-secret", pass)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-basic","token_type":"Bearer","expires_in":60}`))
	}))
	defer server.Close()

	tok, err := auth.ExchangeToken(
		context.Background(),
		server.Client(),
		server.URL,
		auth.TokenEndpointRequest{
			Form: map[string][]string{
				"grant_type": {"client_credentials"},
			},
			BasicAuth: &auth.BasicAuth{
				Username: "client-id",
				Password: "client-secret",
			},
		},
		fixedNow,
	)
	require.NoError(t, err)
	assert.Equal(t, "tok-basic", tok.AccessToken)
}

func TestExchangeTokenOAuth2Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad credentials"}`))
	}))
	defer server.Close()

	_, err := auth.ExchangeToken(
		context.Background(),
		server.Client(),
		server.URL,
		auth.TokenEndpointRequest{
			Form: map[string][]string{"grant_type": {"client_credentials"}},
		},
		fixedNow,
	)
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

	_, err := auth.ExchangeToken(
		context.Background(),
		server.Client(),
		server.URL,
		auth.TokenEndpointRequest{
			Form: map[string][]string{"grant_type": {"client_credentials"}},
		},
		fixedNow,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_token")
}

func TestExchangeTokenDefaultsBearerType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-2"}`))
	}))
	defer server.Close()

	tok, err := auth.ExchangeToken(
		context.Background(),
		server.Client(),
		server.URL,
		auth.TokenEndpointRequest{
			Form: map[string][]string{"grant_type": {"client_credentials"}},
		},
		fixedNow,
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer", tok.TokenType)
}
