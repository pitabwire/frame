package config //nolint:testpackage // test helper for internal tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testOIDCServer struct {
	server *httptest.Server
}

func newTestOIDCServer(t *testing.T, failDiscovery bool, failJWK bool) *testOIDCServer {
	t.Helper()

	svc := &testOIDCServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		if failDiscovery {
			http.Error(w, "discovery failed", http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"jwks_uri":               svc.server.URL + "/.well-known/jwks.json",
			"issuer":                 "http://issuer.local",
			"authorization_endpoint": "http://auth.local",
			"registration_endpoint":  "http://reg.local",
			"token_endpoint":         "http://token.local",
			"userinfo_endpoint":      "http://userinfo.local",
			"revocation_endpoint":    "http://revoke.local",
			"end_session_endpoint":   "http://logout.local",
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		if failJWK {
			http.Error(w, "jwks failed", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"1","use":"sig","n":"x","e":"AQAB","x5c":["cert"]}]}`))
	})

	svc.server = httptest.NewServer(mux)
	t.Cleanup(svc.server.Close)
	return svc
}

func (t *testOIDCServer) discoveryURLRoot() string {
	return strings.TrimSuffix(t.server.URL, "/")
}

func (t *testOIDCServer) jwksURL() string {
	return t.server.URL + "/.well-known/jwks.json"
}
