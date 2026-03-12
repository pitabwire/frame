package signer_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client/oauth2/signer"
	"github.com/pitabwire/frame/config"
)

func TestJWKSURLSigner(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       key,
				KeyID:     "test-kid",
				Use:       "sig",
				Algorithm: "ES256",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	s := signer.NewJWKSURLSigner(server.Client(), &config.PrivateKeyJWTConfig{
		SignerURL: server.URL,
		KeyID:     "test-kid",
	})

	ctx := context.Background()

	alg, err := s.Algorithm(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ES256", alg)

	kid, err := s.KeyID(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-kid", kid)

	sig, err := s.Sign(ctx, []byte("test-payload"))
	require.NoError(t, err)
	assert.NotEmpty(t, sig)
}

func TestJWKSURLSignerNoKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer server.Close()

	s := signer.NewJWKSURLSigner(server.Client(), &config.PrivateKeyJWTConfig{
		SignerURL: server.URL,
	})

	_, err := s.Algorithm(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no keys")
}

func TestJWKSURLSignerServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s := signer.NewJWKSURLSigner(server.Client(), &config.PrivateKeyJWTConfig{
		SignerURL: server.URL,
	})

	_, err := s.Algorithm(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}
