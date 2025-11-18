package openid

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

const (
	defaultJWKSRefreshInterval = 5 * time.Minute
	defaultHTTPClientTimeout   = 5 * time.Second
)

type jwtTokenAuthenticator struct {
	jwtVerificationAudience []string
	jwtVerificationIssuer   string
	tokenAuthenticator      *TokenAuthenticator
}

func (a *jwtTokenAuthenticator) keyFunc(token *jwt.Token) (any, error) {
	if a.tokenAuthenticator == nil {
		return nil, errors.New("token authenticator not initialized")
	}

	return a.tokenAuthenticator.GetKey(token)
}

func NewJwtTokenAuthenticator(cfg config.ConfigurationJWTVerification) security.Authenticator {
	auth := NewTokenAuthenticator(
		cfg.GetOauth2WellKnownJwk(),
		defaultJWKSRefreshInterval,
	)
	auth.Start()

	return &jwtTokenAuthenticator{
		jwtVerificationAudience: cfg.GetVerificationAudience(),
		jwtVerificationIssuer:   cfg.GetVerificationIssuer(),
		tokenAuthenticator:      auth,
	}
}

func (a *jwtTokenAuthenticator) Authenticate(
	ctx context.Context,
	jwtToken string,
	options ...security.AuthOption,
) (context.Context, error) {
	securityOpts := security.AuthOptions{
		Audience:        a.jwtVerificationAudience,
		Issuer:          a.jwtVerificationIssuer,
		DisableSecurity: false,
	}

	for _, opt := range options {
		opt(ctx, &securityOpts)
	}

	claims := &security.AuthenticationClaims{}

	var parseOptions []jwt.ParserOption

	if len(securityOpts.Audience) > 0 {
		parseOptions = append(parseOptions, jwt.WithAudience(securityOpts.Audience...))
	}

	if securityOpts.Issuer != "" {
		parseOptions = append(parseOptions, jwt.WithIssuer(securityOpts.Issuer))
	}

	token, err := jwt.ParseWithClaims(jwtToken, claims, a.keyFunc, parseOptions...)
	if err != nil {
		return ctx, err
	}

	if !token.Valid {
		return ctx, errors.New("supplied token was invalid")
	}

	ctx = security.JwtToContext(ctx, jwtToken)

	ctx = claims.ClaimsToContext(ctx)

	return ctx, nil
}

type JWK struct {
	Kty string `json:"kty"`
	Alg string `json:"alg,omitempty"`
	Use string `json:"use,omitempty"`

	// Common
	Kid string `json:"kid"`

	// RSA
	E string `json:"e,omitempty"`
	N string `json:"n,omitempty"`

	// EC
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`

	// OKP (EdDSA)
	OKPCrv string `json:"crv,omitempty"` //nolint:govet // JWK format allows overlapping field names for different key types
	OKPX   string `json:"x,omitempty"`   //nolint:govet // JWK format allows overlapping field names for different key types
}

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

type TokenAuthenticator struct {
	jwksURL  string
	refresh  time.Duration
	client   *http.Client
	mu       sync.RWMutex
	keys     map[string]any
	lastErr  error
	stopChan chan struct{}
}

// ------------------------------
// Creation
// ------------------------------

func NewTokenAuthenticator(jwksURL string, refresh time.Duration) *TokenAuthenticator {
	a := &TokenAuthenticator{
		jwksURL:  jwksURL,
		refresh:  refresh,
		client:   &http.Client{Timeout: defaultHTTPClientTimeout},
		keys:     make(map[string]any),
		stopChan: make(chan struct{}),
	}
	return a
}

// ------------------------------
// Start / Stop Background Refresh
// ------------------------------

func (a *TokenAuthenticator) Start() {
	// Load immediately
	_ = a.Refresh()

	// Background updater
	go func() {
		ticker := time.NewTicker(a.refresh)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = a.Refresh()
			case <-a.stopChan:
				return
			}
		}
	}()
}

func (a *TokenAuthenticator) Stop() {
	close(a.stopChan)
}

// GetKeyCount returns the number of currently loaded keys (for testing purposes).
func (a *TokenAuthenticator) GetKeyCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.keys)
}

// ------------------------------
// Public Method for JWT Key Lookup
// ------------------------------

func (a *TokenAuthenticator) GetKey(token *jwt.Token) (any, error) {
	if token == nil {
		return nil, errors.New("token is nil")
	}

	kidValue := token.Header["kid"]
	kid, ok := kidValue.(string)
	if !ok || kid == "" {
		return nil, errors.New("token missing kid header or kid not a string")
	}

	a.mu.RLock()
	key := a.keys[kid]
	aErr := a.lastErr
	a.mu.RUnlock()

	if key != nil {
		return key, nil
	}

	// Fallback: reload immediately if previous load failed
	if aErr != nil {
		if err := a.Refresh(); err != nil {
			return nil, fmt.Errorf("refresh failed: %w", err)
		}
	}

	// Try again after refresh
	a.mu.RLock()
	defer a.mu.RUnlock()

	key = a.keys[kid]
	if key == nil {
		return nil, fmt.Errorf("no jwk found for kid %s", kid)
	}
	return key, nil
}

// ------------------------------
// Refresh JWK from Well-Known URL
// ------------------------------

func (a *TokenAuthenticator) Refresh() error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, a.jwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.setErr(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e := fmt.Errorf("jwks fetch returned %d", resp.StatusCode)
		a.setErr(e)
		return e
	}

	var jwks JWKSet
	if decodeErr := json.NewDecoder(resp.Body).Decode(&jwks); decodeErr != nil {
		a.setErr(decodeErr)
		return decodeErr
	}

	keyMap := make(map[string]any)

	for _, k := range jwks.Keys {
		if k.Kid == "" {
			continue
		}

		pub, buildErr := a.buildKey(k)
		if buildErr != nil {
			// Do not fully fail; skip bad key
			continue
		}

		keyMap[k.Kid] = pub
	}

	if len(keyMap) == 0 {
		noKeysErr := errors.New("no valid keys in JWK response")
		a.setErr(noKeysErr)
		return noKeysErr
	}

	a.mu.Lock()
	a.keys = keyMap
	a.lastErr = nil
	a.mu.Unlock()

	return nil
}

// helper.
func (a *TokenAuthenticator) setErr(err error) {
	a.mu.Lock()
	a.lastErr = err
	a.mu.Unlock()
}

// ------------------------------
// Build individual keys (RSA, EC, OKP)
// ------------------------------

func (a *TokenAuthenticator) buildKey(k JWK) (any, error) {
	switch k.Kty {
	case "RSA":
		return buildRSA(k)
	case "EC":
		return buildEC(k)
	case "OKP":
		return buildOKP(k)
	default:
		return nil, fmt.Errorf("unsupported JWK kty=%s", k.Kty)
	}
}

// ------------------------------
// RSA
// ------------------------------

func buildRSA(k JWK) (*rsa.PublicKey, error) {
	if k.N == "" || k.E == "" {
		return nil, errors.New("RSA key missing n or e")
	}

	nb, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA modulus: %w", err)
	}

	eb, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA exponent: %w", err)
	}

	exp := big.NewInt(0).SetBytes(eb).Int64()
	if exp <= 0 {
		return nil, fmt.Errorf("invalid RSA exponent %d", exp)
	}

	return &rsa.PublicKey{
		N: big.NewInt(0).SetBytes(nb),
		E: int(exp),
	}, nil
}

// ------------------------------
// EC (P-256, P-384, P-521)
// ------------------------------

func buildEC(k JWK) (*ecdsa.PublicKey, error) {
	if k.X == "" || k.Y == "" {
		return nil, errors.New("EC key missing X or Y")
	}

	xb, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("invalid EC x: %w", err)
	}
	yb, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("invalid EC y: %w", err)
	}

	var curve elliptic.Curve
	switch k.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported EC curve %s", k.Crv)
	}

	x := new(big.Int).SetBytes(xb)
	y := new(big.Int).SetBytes(yb)

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// ------------------------------
// OKP (Ed25519)
// ------------------------------

func buildOKP(k JWK) (any, error) {
	if k.OKPX == "" || k.OKPCrv != "Ed25519" {
		return nil, errors.New("unsupported OKP key or curve")
	}

	pk, err := base64.RawURLEncoding.DecodeString(k.OKPX)
	if err != nil {
		return nil, fmt.Errorf("invalid OKP x: %w", err)
	}
	if len(pk) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key size")
	}
	return ed25519.PublicKey(pk), nil
}
