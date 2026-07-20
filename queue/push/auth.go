package push

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/pitabwire/frame/v2/queue"
)

// AuthMode is the push endpoint authentication mode.
type AuthMode string

const (
	AuthNone   AuthMode = "none"
	AuthBearer AuthMode = "bearer"
	AuthOIDC   AuthMode = "oidc"
)

// Authenticator validates push HTTP requests.
type Authenticator interface {
	Authenticate(r *http.Request) error
}

// NoneAuth allows all requests (local / private mesh only).
type NoneAuth struct{}

func (NoneAuth) Authenticate(_ *http.Request) error { return nil }

// BearerAuth checks Authorization: Bearer <token> with constant-time compare.
type BearerAuth struct {
	Token string
}

func (b BearerAuth) Authenticate(r *http.Request) error {
	if b.Token == "" {
		return queue.ErrUnauthorized
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return queue.ErrUnauthorized
	}
	got := h[len(prefix):]
	if subtle.ConstantTimeCompare([]byte(got), []byte(b.Token)) != 1 {
		return queue.ErrUnauthorized
	}
	return nil
}

// ParseAuthMode normalizes a config string to AuthMode.
func ParseAuthMode(s string) AuthMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bearer":
		return AuthBearer
	case "oidc":
		return AuthOIDC
	default:
		return AuthNone
	}
}
