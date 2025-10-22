package client

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const defaultHTTPTimeoutSeconds = 30

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

// NewHTTPClient creates a new HTTP client with the provided options.
// If no transport is specified, it defaults to otelhttp.NewTransport(http.DefaultTransport).
func NewHTTPClient(opts ...HTTPOption) *http.Client {
	config := &httpConfig{}
	for _, opt := range opts {
		opt(config)
	}

	if config.timeout == 0 {
		config.timeout = time.Duration(defaultHTTPTimeoutSeconds) * time.Second
	}

	// Set defaults
	if config.transport == nil {
		config.transport = http.DefaultTransport
	}

	transport := otelhttp.NewTransport(config.transport)

	client := &http.Client{
		Transport:     transport,
		Timeout:       config.timeout,
		Jar:           config.jar,
		CheckRedirect: config.checkRedirect,
	}

	if config.idleTimeout > 0 {
		if t, ok := client.Transport.(*http.Transport); ok {
			t.IdleConnTimeout = config.idleTimeout
		}
	}

	return client
}
