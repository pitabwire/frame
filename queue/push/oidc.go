package push

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/security/openid"
)

const (
	defaultGoogleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"
	defaultJWKSRefresh   = time.Hour
	defaultClockSkew     = time.Minute
)

// OIDCConfig configures dedicated Google-oriented push OIDC validation.
type OIDCConfig struct {
	Audience    string
	Issuers     []string
	JWKSURL     string
	JWKSRefresh time.Duration
}

// GoogleCloudTasksOIDCPreset returns defaults for Cloud Tasks OIDC tokens.
func GoogleCloudTasksOIDCPreset(audience string) OIDCConfig {
	return OIDCConfig{
		Audience: audience,
		Issuers: []string{
			"https://accounts.google.com",
			"accounts.google.com",
		},
		JWKSURL:     defaultGoogleJWKSURL,
		JWKSRefresh: defaultJWKSRefresh,
	}
}

// OIDCAuth validates Google OIDC bearer tokens for Cloud Tasks push.
type OIDCAuth struct {
	cfg  OIDCConfig
	auth *openid.TokenAuthenticator
}

// NewOIDCAuth creates an OIDC authenticator and starts JWKS refresh.
func NewOIDCAuth(ctx context.Context, cfg OIDCConfig) *OIDCAuth {
	if cfg.JWKSURL == "" {
		cfg.JWKSURL = defaultGoogleJWKSURL
	}
	if cfg.JWKSRefresh <= 0 {
		cfg.JWKSRefresh = defaultJWKSRefresh
	}
	if len(cfg.Issuers) == 0 {
		cfg.Issuers = GoogleCloudTasksOIDCPreset(cfg.Audience).Issuers
	}
	ta := openid.NewTokenAuthenticator(cfg.JWKSURL, cfg.JWKSRefresh)
	ta.Start(ctx)
	return &OIDCAuth{cfg: cfg, auth: ta}
}

// Stop stops JWKS refresh.
func (o *OIDCAuth) Stop() {
	if o != nil && o.auth != nil {
		o.auth.Stop()
	}
}

func (o *OIDCAuth) Authenticate(r *http.Request) error {
	if o == nil || o.auth == nil {
		return queue.ErrUnauthorized
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return queue.ErrUnauthorized
	}
	raw := strings.TrimSpace(h[len(prefix):])
	if raw == "" {
		return queue.ErrUnauthorized
	}

	audience := o.cfg.Audience
	if audience == "" {
		// Cloud Tasks default audience is the target URL without query/fragment.
		audience = absoluteURL(r)
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, o.auth.GetKey,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(defaultClockSkew),
	)
	if err != nil {
		return fmt.Errorf("%w: %w", queue.ErrUnauthorized, err)
	}
	if !token.Valid {
		return queue.ErrUnauthorized
	}

	iss, _ := claims["iss"].(string)
	if !issuerAllowed(iss, o.cfg.Issuers) {
		return fmt.Errorf("%w: issuer %q", queue.ErrForbidden, iss)
	}

	if !audienceMatch(claims["aud"], audience) {
		return fmt.Errorf("%w: audience mismatch", queue.ErrForbidden)
	}
	return nil
}

func issuerAllowed(iss string, allowed []string) bool {
	for _, a := range allowed {
		if iss == a {
			return true
		}
	}
	return false
}

func audienceMatch(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == want {
				return true
			}
		}
	}
	return false
}

func absoluteURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return scheme + "://" + host + r.URL.Path
}
