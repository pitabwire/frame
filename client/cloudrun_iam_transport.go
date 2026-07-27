package client

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// headerServerlessAuthorization is Cloud Run's dual-auth header: product OAuth
// stays in Authorization; the Google ID token for roles/run.invoker goes here.
// See https://cloud.google.com/run/docs/authenticating/service-to-service
const headerServerlessAuthorization = "X-Serverless-Authorization"

// cloudRunIAMTransport attaches a Google ID token as X-Serverless-Authorization
// for HTTPS requests so IAM-authenticated Cloud Run services accept product
// OAuth in Authorization. No-op when the metadata server is unavailable
// (local / in-cluster), or when the header is already set.
//
// Token sources are cached per audience (scheme://host) so each host reuses
// one idtoken.TokenSource for the process lifetime.
type cloudRunIAMTransport struct {
	inner http.RoundTripper

	mu    sync.Mutex
	cache map[string]oauth2.TokenSource
}

func (t *cloudRunIAMTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return t.base().RoundTrip(req)
	}
	// Never overwrite an explicit dual-auth header from the caller.
	if req.Header.Get(headerServerlessAuthorization) != "" {
		return t.base().RoundTrip(req)
	}
	if !shouldAttachCloudRunIDToken(req.URL) {
		return t.base().RoundTrip(req)
	}

	audience := requestAudience(req.URL)
	ts := t.tokenSource(audience)
	if ts == nil {
		return t.base().RoundTrip(req)
	}
	tok, err := ts.Token()
	if err != nil || tok == nil || tok.AccessToken == "" {
		// Metadata unavailable or audience rejected — fall through with OAuth only.
		return t.base().RoundTrip(req)
	}

	// Clone so concurrent callers (and retries) do not race on Header.
	req2 := req.Clone(req.Context())
	req2.Header.Set(headerServerlessAuthorization, "Bearer "+tok.AccessToken)
	return t.base().RoundTrip(req2)
}

func (t *cloudRunIAMTransport) base() http.RoundTripper {
	if t.inner != nil {
		return t.inner
	}
	return http.DefaultTransport
}

func (t *cloudRunIAMTransport) CloseIdleConnections() {
	if closer, ok := t.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t *cloudRunIAMTransport) tokenSource(audience string) oauth2.TokenSource {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = make(map[string]oauth2.TokenSource)
	}
	if ts, ok := t.cache[audience]; ok {
		return ts
	}
	ts, err := idtoken.NewTokenSource(context.Background(), audience)
	if err != nil {
		// Cache nil so we do not thrash metadata on every request off-GCP.
		t.cache[audience] = nil
		return nil
	}
	t.cache[audience] = ts
	return ts
}

func shouldAttachCloudRunIDToken(u *url.URL) bool {
	if u == nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	// Plain cluster DNS stays on product OAuth only.
	if strings.HasSuffix(host, ".svc") || strings.HasSuffix(host, ".svc.cluster.local") {
		return false
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return false
	}
	return true
}

func requestAudience(u *url.URL) string {
	// Cloud Run custom audiences are typically https://host (no path).
	return "https://" + u.Host
}
