package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"

	"github.com/pitabwire/frame/config"
)

const maxJWKSResponseBytes = 512 << 10 // 512 KiB

// JWKSURLSigner fetches a JWK set from a URL and uses the first signing key
// to produce JWT signatures. This is suitable for setups where an admin API
// (e.g. Ory Hydra) manages the key pairs and exposes them via JWKS.
type JWKSURLSigner struct {
	httpClient *http.Client
	jwksURL    string
	keyID      string

	mu     sync.Mutex
	cached *resolvedKey
}

type resolvedKey struct {
	signer crypto.Signer
	kid    string
	alg    string
}

// NewJWKSURLSigner creates a signer that fetches signing keys from a JWKS URL.
func NewJWKSURLSigner(httpClient *http.Client, cfg *config.PrivateKeyJWTConfig) *JWKSURLSigner {
	return &JWKSURLSigner{
		httpClient: httpClient,
		jwksURL:    strings.TrimSpace(cfg.SignerURL),
		keyID:      strings.TrimSpace(cfg.KeyID),
	}
}

func (s *JWKSURLSigner) Algorithm(ctx context.Context) (string, error) {
	key, err := s.resolveKey(ctx)
	if err != nil {
		return "", err
	}

	return key.alg, nil
}

func (s *JWKSURLSigner) KeyID(ctx context.Context) (string, error) {
	key, err := s.resolveKey(ctx)
	if err != nil {
		return "", err
	}

	return key.kid, nil
}

func (s *JWKSURLSigner) Sign(ctx context.Context, payload []byte) ([]byte, error) {
	key, err := s.resolveKey(ctx)
	if err != nil {
		return nil, err
	}

	method := signingMethodForCryptoKey(key.signer)
	if method == nil {
		return nil, fmt.Errorf("unsupported key type %T", key.signer)
	}

	sig, err := method.Sign(string(payload), key.signer)
	if err != nil {
		return nil, fmt.Errorf("sign payload: %w", err)
	}

	return sig, nil
}

func (s *JWKSURLSigner) resolveKey(ctx context.Context) (*resolvedKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cached != nil {
		return s.cached, nil
	}

	key, err := s.fetchAndSelectKey(ctx)
	if err != nil {
		return nil, err
	}

	s.cached = key

	return key, nil
}

func (s *JWKSURLSigner) fetchAndSelectKey(ctx context.Context) (*resolvedKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build JWKS request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS from %s: %w", s.jwksURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read JWKS response: %w", err)
	}

	var jwks jose.JSONWebKeySet
	if parseErr := json.Unmarshal(body, &jwks); parseErr != nil {
		return nil, fmt.Errorf("parse JWKS: %w", parseErr)
	}

	return selectSigningKey(jwks.Keys, s.keyID)
}

func selectSigningKey(keys []jose.JSONWebKey, preferredKID string) (*resolvedKey, error) {
	if len(keys) == 0 {
		return nil, errors.New("JWKS contains no keys")
	}

	for _, key := range keys {
		if key.Use != "" && key.Use != "sig" {
			continue
		}

		if preferredKID != "" && key.KeyID != preferredKID {
			continue
		}

		signer, ok := key.Key.(crypto.Signer)
		if !ok {
			continue
		}

		method := signingMethodForCryptoKey(signer)
		if method == nil {
			continue
		}

		return &resolvedKey{
			signer: signer,
			kid:    key.KeyID,
			alg:    method.Alg(),
		}, nil
	}

	// If preferred KID didn't match, try any signing key.
	if preferredKID != "" {
		return selectSigningKey(keys, "")
	}

	return nil, errors.New("JWKS contains no usable signing keys")
}

func signingMethodForCryptoKey(key crypto.Signer) jwt.SigningMethod {
	switch key.(type) {
	case *rsa.PrivateKey:
		return jwt.SigningMethodRS256
	case *ecdsa.PrivateKey:
		return jwt.SigningMethodES256
	case ed25519.PrivateKey:
		return jwt.SigningMethodEdDSA
	default:
		return nil
	}
}
