package client

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	defaultHTTPTimeoutSeconds     = 30
	defaultHTTPIdleTimeoutSeconds = 90
)

// HTTPOption configures HTTP client behavior.
// It can be used to configure timeout, transport, and other HTTP client settings.
type HTTPOption func(*httpConfig)

// httpConfig holds HTTP client configuration.
type httpConfig struct {
	timeout       time.Duration
	transport     http.RoundTripper
	jar           http.CookieJar
	checkRedirect func(req *http.Request, via []*http.Request) error
	idleTimeout   time.Duration
	enableH2C     bool

	cliCredCfg *clientcredentials.Config

	traceRequests       bool
	traceRequestHeaders bool
}

// WithHTTPTimeout sets the request timeout.
func WithHTTPTimeout(timeout time.Duration) HTTPOption {
	return func(c *httpConfig) {
		c.timeout = timeout
	}
}

// WithHTTPTransport sets the HTTP transport.
func WithHTTPTransport(transport http.RoundTripper) HTTPOption {
	return func(c *httpConfig) {
		c.transport = transport
	}
}

// WithHTTPCookieJar sets the cookie jar.
func WithHTTPCookieJar(jar http.CookieJar) HTTPOption {
	return func(c *httpConfig) {
		c.jar = jar
	}
}

// WithHTTPCheckRedirect sets the redirect policy.
func WithHTTPCheckRedirect(checkRedirect func(req *http.Request, via []*http.Request) error) HTTPOption {
	return func(c *httpConfig) {
		c.checkRedirect = checkRedirect
	}
}

// WithHTTPIdleTimeout sets the idle timeout.
func WithHTTPIdleTimeout(timeout time.Duration) HTTPOption {
	return func(c *httpConfig) {
		c.idleTimeout = timeout
	}
}

// WithHTTPEnableH2C sets the enable h2c option to active.
func WithHTTPEnableH2C() HTTPOption {
	return func(c *httpConfig) {
		c.enableH2C = true
	}
}

// WithHTTPClientCredentials the client credentials the client can utilize.
func WithHTTPClientCredentials(cfg *clientcredentials.Config) HTTPOption {
	return func(c *httpConfig) {
		c.cliCredCfg = cfg
	}
}

// WithHTTPTraceRequests enables or disables request logging.
func WithHTTPTraceRequests() HTTPOption {
	return func(c *httpConfig) {
		c.traceRequests = true
	}
}

// WithHTTPTraceRequestHeaders enables or disables header logging.
func WithHTTPTraceRequestHeaders() HTTPOption {
	return func(c *httpConfig) {
		c.traceRequestHeaders = true
	}
}

// NewHTTPClient creates a new HTTP client with the provided options.
// If no transport is specified, it defaults to otelhttp.NewTransport(http.DefaultTransport).
func NewHTTPClient(ctx context.Context, opts ...HTTPOption) *http.Client {
	cfg := &httpConfig{
		timeout:     time.Duration(defaultHTTPTimeoutSeconds) * time.Second,
		idleTimeout: time.Duration(defaultHTTPIdleTimeoutSeconds) * time.Second,
		transport:   http.DefaultTransport,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	base := cfg.transport

	// Enable H2C if desired
	if cfg.enableH2C {
		if t, ok := base.(*http.Transport); ok {
			protocols := new(http.Protocols)
			protocols.SetUnencryptedHTTP2(true)
			t.Protocols = protocols
		}
	}

	// Add OpenTelemetry wrapper once
	if _, ok := base.(*otelhttp.Transport); !ok {
		base = otelhttp.NewTransport(base)
	}

	// Optional: request/response logging
	if cfg.traceRequests {
		base = NewLoggingTransport(base,
			WithTransportLogRequests(true),
			WithTransportLogResponses(true),
			WithTransportLogHeaders(cfg.traceRequestHeaders),
			WithTransportLogBody(true))
	}

	client := &http.Client{
		Transport:     base,
		Timeout:       cfg.timeout,
		Jar:           cfg.jar,
		CheckRedirect: cfg.checkRedirect,
	}

	if cfg.cliCredCfg != nil {
		oauth2Ctx := context.WithValue(ctx, oauth2.HTTPClient, client)
		// Get the OAuth2 client and preserve our transport configuration
		client = cfg.cliCredCfg.Client(oauth2Ctx)
	}

	if t, ok := base.(*http.Transport); ok && cfg.idleTimeout > 0 {
		t.IdleConnTimeout = cfg.idleTimeout
	}

	return client
}
