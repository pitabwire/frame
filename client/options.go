package client

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
func NewHTTPClient(opts ...HTTPOption) *http.Client {
	cfg := &httpConfig{
		timeout:     time.Duration(defaultHTTPTimeoutSeconds) * time.Second,
		idleTimeout: time.Duration(defaultHTTPIdleTimeoutSeconds) * time.Second,
		transport:   otelhttp.NewTransport(http.DefaultTransport),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.traceRequests {
		cfg.transport = NewLoggingTransport(cfg.transport,
			WithTransportLogRequests(true),
			WithTransportLogResponses(true),
			WithTransportLogHeaders(cfg.traceRequestHeaders),
			WithTransportLogBody(true))
	}

	client := &http.Client{
		Transport:     cfg.transport,
		Timeout:       cfg.timeout,
		Jar:           cfg.jar,
		CheckRedirect: cfg.checkRedirect,
	}

	if cfg.idleTimeout > 0 {
		if t, ok := client.Transport.(*http.Transport); ok {
			t.IdleConnTimeout = cfg.idleTimeout
		}
	}

	return client
}
