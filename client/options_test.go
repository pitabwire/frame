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

	"github.com/pitabwire/frame/config"
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
