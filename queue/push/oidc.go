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
	// AllowedEmails restricts accepted principals when non-empty. Matched
	// case-insensitively against the JWT email claim, then sub. Empty means
	// no principal allowlist (signature + issuer + audience only).
	AllowedEmails []string
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

// ParseCommaSeparatedList splits a comma-separated config string, trims entries,
// and drops empties. Used for OIDC issuers and allowed emails.
func ParseCommaSeparatedList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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

	if len(o.cfg.AllowedEmails) > 0 {
		email, sub := principalFromClaims(claims)
		if !principalAllowed(email, sub, o.cfg.AllowedEmails) {
			return fmt.Errorf("%w: service account not allowed", queue.ErrForbidden)
		}
	}
	return nil
}

func principalFromClaims(claims jwt.MapClaims) (string, string) {
	var email, sub string
	if v, ok := claims["email"].(string); ok {
		email = strings.TrimSpace(v)
	}
	if v, ok := claims["sub"].(string); ok {
		sub = strings.TrimSpace(v)
	}
	return email, sub
}

// principalAllowed reports whether email or sub matches an allowlist entry
// (case-insensitive). Empty allowlist is treated as unrestricted by the caller.
func principalAllowed(email, sub string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	email = strings.ToLower(email)
	sub = strings.ToLower(sub)
	for _, a := range allowed {
		want := strings.ToLower(strings.TrimSpace(a))
		if want == "" {
			continue
		}
		if email != "" && email == want {
			return true
		}
		if sub != "" && sub == want {
			return true
		}
	}
	return false
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
