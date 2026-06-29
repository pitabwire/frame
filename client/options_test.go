package client //nolint:testpackage // tests access unexported httpConfig

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/pitabwire/frame/v2/config"
)

type OptionsSuite struct {
	suite.Suite
}

type workloadAPIConfig struct {
	trustDomain string
}

func (w workloadAPIConfig) GetTrustedDomain() string {
	return w.trustDomain
}

func TestOptionsSuite(t *testing.T) {
	suite.Run(t, new(OptionsSuite))
}

func (s *OptionsSuite) TestProcessAndOptionFunctions() {
	jar, err := cookiejar.New(nil)
	s.Require().NoError(err)

	redirect := func(_ *http.Request, _ []*http.Request) error { return nil }
	retry := &RetryPolicy{
		MaxAttempts: 7,
		Backoff: func(_ int) time.Duration {
			return time.Second
		},
	}

	cfg := &httpConfig{}
	cfg.process(context.Background(),
		WithHTTPTimeout(3*time.Second),
		WithHTTPTransport(http.DefaultTransport),
		WithHTTPCookieJar(jar),
		WithHTTPCheckRedirect(redirect),
		WithHTTPIdleTimeout(11*time.Second),
		WithHTTPEnableH2C(),
		WithHTTPClientCredentials(&clientcredentials.Config{}),
		WithHTTPTraceRequests(),
		WithHTTPTraceRequestHeaders(),
		WithHTTPRetryPolicy(retry),
	)

	s.Equal(3*time.Second, cfg.timeout)
	s.Equal(http.DefaultTransport, cfg.transport)
	s.Equal(jar, cfg.jar)
	s.NotNil(cfg.checkRedirect)
	s.Equal(11*time.Second, cfg.idleTimeout)
	s.True(cfg.enableH2C)
	s.NotNil(cfg.cliCredCfg)
	s.True(cfg.traceRequests)
	s.True(cfg.traceRequestHeaders)
	s.Equal(retry, cfg.retryPolicy)
}

func (s *OptionsSuite) TestProcessSetsDefaultRetryPolicy() {
	cfg := &httpConfig{}
	cfg.process(context.Background())
	s.NotNil(cfg.retryPolicy)
	s.Equal(defaultMaxRetryAttempts, cfg.retryPolicy.MaxAttempts)
	s.Greater(cfg.retryPolicy.Backoff(2), time.Duration(0))
}

func (s *OptionsSuite) TestNewHTTPClient() {
	tr := &http.Transport{}
	client := NewHTTPClient(context.Background(),
		WithHTTPTimeout(2*time.Second),
		WithHTTPTransport(tr),
		WithHTTPIdleTimeout(8*time.Second),
		WithHTTPEnableH2C(),
		WithHTTPTraceRequests(),
		WithHTTPTraceRequestHeaders(),
	)

	s.NotNil(client)
	s.Equal(2*time.Second, client.Timeout)
	s.NotNil(client.Transport)
	s.Nil(tr.Protocols)
}

func (s *OptionsSuite) TestWithHTTPNoAuthSetsFlag() {
	cfg := &httpConfig{}
	cfg.process(context.Background(), WithHTTPNoAuth())
	s.True(cfg.noAuth)
}

// TestWithHTTPNoAuthSkipsOAuth proves the no-auth client bypasses all outbound
// OAuth: even with client credentials configured (whose token fetch would fail
// against an unreachable token URL), the request goes through unwrapped and the
// caller's own Authorization header survives — the external-API-key use case.
func (s *OptionsSuite) TestWithHTTPNoAuthSkipsOAuth() {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer server.Close()

	client, err := newHTTPClient(context.Background(),
		WithHTTPClientCredentials(&clientcredentials.Config{TokenURL: "http://127.0.0.1:1/token"}),
		WithHTTPNoAuth(),
	)
	s.Require().NoError(err)

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	s.Require().NoError(err)
	req.Header.Set("Authorization", "Bearer external-api-key")
	resp, err := client.Do(req)
	s.Require().NoError(err) // would fail the token fetch if OAuth weren't skipped
	_ = resp.Body.Close()
	s.Equal("Bearer external-api-key", gotAuth)
}

// httpClientTimeoutCfg satisfies config.ConfigurationHTTPClient for tests
// that exercise the in-context timeout seeding path in newHTTPClient.
type httpClientTimeoutCfg struct {
	timeout     time.Duration
	idleTimeout time.Duration
}

func (h httpClientTimeoutCfg) GetHTTPClientTimeout() time.Duration {
	return h.timeout
}

func (h httpClientTimeoutCfg) GetHTTPClientIdleTimeout() time.Duration {
	return h.idleTimeout
}

// contextWithHTTPClientTimeout is a tiny helper for tests that need a
// context carrying a ConfigurationHTTPClient with the given timeout
// (idle timeout left at its default).
func contextWithHTTPClientTimeout(timeout time.Duration) context.Context {
	return config.ToContext(context.Background(), httpClientTimeoutCfg{
		timeout: timeout,
	})
}

func (s *OptionsSuite) TestNewHTTPClientTakesTimeoutFromContextConfig() {
	ctx := config.ToContext(context.Background(), httpClientTimeoutCfg{
		timeout:     5 * time.Minute,
		idleTimeout: 2 * time.Minute,
	})

	client := NewHTTPClient(ctx)

	s.NotNil(client)
	s.Equal(5*time.Minute, client.Timeout)
}

func (s *OptionsSuite) TestNewHTTPClientExplicitOptionWinsOverConfig() {
	ctx := config.ToContext(context.Background(), httpClientTimeoutCfg{
		timeout:     5 * time.Minute,
		idleTimeout: 2 * time.Minute,
	})

	// Explicit WithHTTPTimeout must take precedence over the config-seeded
	// value so callers that have a specific deadline are honoured.
	client := NewHTTPClient(ctx, WithHTTPTimeout(7*time.Second))

	s.NotNil(client)
	s.Equal(7*time.Second, client.Timeout)
}

func (s *OptionsSuite) TestNewHTTPClientFallsBackToDefaultWithoutConfig() {
	client := NewHTTPClient(context.Background())

	s.NotNil(client)
	s.Equal(time.Duration(defaultHTTPTimeoutSeconds)*time.Second, client.Timeout)
}

func (s *OptionsSuite) TestProcessPicksWorkloadAPITrustDomainFromContext() {
	ctx := config.ToContext(context.Background(), workloadAPIConfig{
		trustDomain: "example.org",
	})

	cfg := &httpConfig{}
	cfg.process(ctx)

	s.Equal("example.org", cfg.workloadAPITrustDomain)
	s.False(cfg.workloadAPIRequested)
}

func (s *OptionsSuite) TestProcessBuildsWorkloadAPITargetIDFromPath() {
	ctx := config.ToContext(context.Background(), workloadAPIConfig{
		trustDomain: "example.org",
	})

	cfg := &httpConfig{}
	cfg.process(ctx, WithHTTPWorkloadAPITargetPath("/ns/backend/sa/payments-api"))

	s.Equal("spiffe://example.org/ns/backend/sa/payments-api", cfg.workloadAPITargetID)
}

func (s *OptionsSuite) TestProcessUsesExplicitWorkloadAPITargetID() {
	ctx := config.ToContext(context.Background(), workloadAPIConfig{
		trustDomain: "example.org",
	})

	cfg := &httpConfig{}
	cfg.process(
		ctx,
		WithHTTPWorkloadAPITargetPath("/ns/backend/sa/default"),
		WithHTTPWorkloadAPITargetID("spiffe://example.org/ns/backend/sa/payments-api"),
	)

	s.Equal("spiffe://example.org/ns/backend/sa/payments-api", cfg.workloadAPITargetID)
}

func (s *OptionsSuite) TestPrepareBaseTransportRejectsWorkloadAPIOnWrappedTransport() {
	cfg := &httpConfig{
		transport:            NewLoggingTransport(http.DefaultTransport),
		workloadAPIRequested: true,
		workloadAPITargetID:  "spiffe://example.org/ns/backend/sa/payments-api",
	}

	_, _, err := prepareBaseTransport(context.Background(), cfg)
	s.Require().ErrorIs(err, ErrWorkloadAPITransportUnsupported)
}

func (s *OptionsSuite) TestPrepareBaseTransportRejectsWorkloadAPIWithH2C() {
	cfg := &httpConfig{
		transport:            &http.Transport{},
		enableH2C:            true,
		workloadAPIRequested: true,
		workloadAPITargetID:  "spiffe://example.org/ns/backend/sa/payments-api",
	}

	_, _, err := prepareBaseTransport(context.Background(), cfg)
	s.Require().ErrorIs(err, ErrWorkloadAPIH2CIncompatible)
}

func (s *OptionsSuite) TestProcessSkipsWorkloadAPITargetPathWhenTrustDomainAbsent() {
	cfg := &httpConfig{}
	cfg.process(context.Background(), WithHTTPWorkloadAPITargetPath("/ns/backend/sa/payments-api"))

	s.False(cfg.workloadAPIRequested)
	s.Empty(cfg.workloadAPITargetPath)
	s.Empty(cfg.workloadAPITargetID)
}

func (s *OptionsSuite) TestNewHTTPClientWorksWhenTargetPathSetWithoutTrustDomain() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(context.Background(),
		WithHTTPWorkloadAPITargetPath("/ns/backend/sa/payments-api"),
	)

	resp, err := client.Get(server.URL)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.NoError(resp.Body.Close())
}

func (s *OptionsSuite) TestNewHTTPClientFailsClosedForInvalidWorkloadAPITargetID() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := newHTTPClient(context.Background(), WithHTTPWorkloadAPITargetID("not-a-spiffe-id"))

	s.Require().Error(err)
	s.Require().NotNil(client)

	resp, reqErr := client.Get(server.URL)
	s.Nil(resp)
	s.Require().Error(reqErr)
}

func (s *OptionsSuite) TestNewHTTPClientFailsClosedWhenWorkloadAPISourceUnavailable() {
	s.T().Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/frame-spiffe-missing.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ctx = config.ToContext(ctx, workloadAPIConfig{
		trustDomain: "example.org",
	})

	client, err := newHTTPClient(ctx, WithHTTPWorkloadAPITargetPath("/ns/backend/sa/payments-api"))

	s.Require().Error(err)
	s.Require().NotNil(client)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, reqErr := client.Get(server.URL)
	s.Nil(resp)
	s.Require().Error(reqErr)
	s.Require().ErrorIs(reqErr, err)
}

func (s *OptionsSuite) TestNewHTTPClientDoesNotAutoEnableWorkloadAPIFromContext() {
	s.T().Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/frame-spiffe-missing.sock")

	ctx := config.ToContext(context.Background(), workloadAPIConfig{
		trustDomain: "example.org",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(ctx)
	resp, err := client.Get(server.URL)

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(http.StatusNoContent, resp.StatusCode)
	s.NoError(resp.Body.Close())
}
