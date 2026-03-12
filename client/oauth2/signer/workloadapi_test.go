package signer_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client/oauth2/signer"
	"github.com/pitabwire/frame/config"
)

func stubFetchSVIDs(t *testing.T, svids []*x509svid.SVID) {
	t.Helper()
	restore := signer.SetFetchX509SVIDsForTest(
		func(context.Context, ...workloadapi.ClientOption) ([]*x509svid.SVID, error) {
			return svids, nil
		},
	)
	t.Cleanup(restore)
}

func TestWorkloadAPISignerAlgorithmRSA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	stubFetchSVIDs(t, []*x509svid.SVID{{
		ID:         spiffeid.RequireFromString("spiffe://example.org/svc"),
		PrivateKey: key,
	}})

	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		Source: config.PrivateKeyJWTSourceWorkloadAPI,
	})

	alg, err := s.Algorithm(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "RS256", alg)
}

func TestWorkloadAPISignerAlgorithmECDSA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	stubFetchSVIDs(t, []*x509svid.SVID{{
		ID:         spiffeid.RequireFromString("spiffe://example.org/svc"),
		PrivateKey: key,
	}})

	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		Source: config.PrivateKeyJWTSourceWorkloadAPI,
	})

	alg, err := s.Algorithm(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ES256", alg)
}

func TestWorkloadAPISignerAlgorithmEdDSA(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	stubFetchSVIDs(t, []*x509svid.SVID{{
		ID:         spiffeid.RequireFromString("spiffe://example.org/svc"),
		PrivateKey: key,
	}})

	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		Source: config.PrivateKeyJWTSourceWorkloadAPI,
	})

	alg, err := s.Algorithm(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "EdDSA", alg)
}

func TestWorkloadAPISignerKeyID(t *testing.T) {
	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		KeyID: "my-key-id",
	})

	kid, err := s.KeyID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-key-id", kid)
}

func TestWorkloadAPISignerSign(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	stubFetchSVIDs(t, []*x509svid.SVID{{
		ID:         spiffeid.RequireFromString("spiffe://example.org/svc"),
		PrivateKey: key,
	}})

	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		Source: config.PrivateKeyJWTSourceWorkloadAPI,
	})

	payload := []byte("test.payload")
	sig, err := s.Sign(context.Background(), payload)
	require.NoError(t, err)
	require.NotEmpty(t, sig)
}

func TestWorkloadAPISignerSelectsBySPIFFEID(t *testing.T) {
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	stubFetchSVIDs(t, []*x509svid.SVID{
		{
			ID:         spiffeid.RequireFromString("spiffe://example.org/svc-a"),
			PrivateKey: key1,
		},
		{
			ID:         spiffeid.RequireFromString("spiffe://example.org/svc-b"),
			PrivateKey: key2,
		},
	})

	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		SPIFFEID: "spiffe://example.org/svc-b",
	})

	alg, err := s.Algorithm(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ES256", alg)
}

func TestWorkloadAPISignerSelectsByHint(t *testing.T) {
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	stubFetchSVIDs(t, []*x509svid.SVID{
		{
			ID:         spiffeid.RequireFromString("spiffe://example.org/svc-a"),
			Hint:       "alpha",
			PrivateKey: key1,
		},
		{
			ID:         spiffeid.RequireFromString("spiffe://example.org/svc-b"),
			Hint:       "beta",
			PrivateKey: key2,
		},
	})

	s := signer.NewWorkloadAPISigner(&config.PrivateKeyJWTConfig{
		Hint: "beta",
	})

	alg, err := s.Algorithm(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ES256", alg)
}

func TestResolveWorkloadAPIFromSource(t *testing.T) {
	s, err := signer.Resolve(context.Background(), nil, &config.PrivateKeyJWTConfig{
		Source: config.PrivateKeyJWTSourceWorkloadAPI,
	})
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestResolveWorkloadAPIFromSPIFFEID(t *testing.T) {
	s, err := signer.Resolve(context.Background(), nil, &config.PrivateKeyJWTConfig{
		SPIFFEID: "spiffe://example.org/svc",
	})
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestResolveRejectsNilConfig(t *testing.T) {
	_, err := signer.Resolve(context.Background(), nil, nil)
	require.Error(t, err)
}

func TestResolveRejectsUnsupportedSource(t *testing.T) {
	_, err := signer.Resolve(context.Background(), nil, &config.PrivateKeyJWTConfig{
		PrivateKeyPath: "/tmp/key.pem",
	})
	require.Error(t, err)
}
