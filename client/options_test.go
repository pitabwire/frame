package client //nolint:testpackage // tests access unexported httpConfig

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2/clientcredentials"
)

type OptionsSuite struct {
	suite.Suite
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
	cfg.process(
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
	cfg.process()
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
	// H2C mutates the base transport before wrapping.
	s.NotNil(tr.Protocols)
}
