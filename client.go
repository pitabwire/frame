package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

// InvokeRestService convenience method to call a http endpoint and utilize the raw results.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *Service) InvokeRestService(ctx context.Context,
	method string, endpointURL string, payload map[string]any,
	headers map[string][]string, opts ...HTTPOption) (int, []byte, error) {
	if headers == nil {
		headers = map[string][]string{
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		}
	}

	var body io.Reader
	if payload != nil {
		postBody, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}

		body = bytes.NewBuffer(postBody)
	}

	// Apply options
	config := &httpConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Apply timeout if specified
	if config.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, body)
	if err != nil {
		return 0, nil, err
	}

	req.Header = headers

	cfg, ok := s.Config().(ConfigurationLogLevel)
	if ok && cfg.LoggingLevelIsDebug() {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		s.Log(ctx).WithField("request", string(reqDump)).Debug("request out")
	}

	//nolint:bodyclose //this is done by util.CloseAndLogOnError()
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer util.CloseAndLogOnError(ctx, resp.Body)

	if ok && cfg.LoggingLevelIsDebug() {
		respDump, _ := httputil.DumpResponse(resp, true)
		s.Log(ctx).WithField("response", string(respDump)).Debug("response in")
	}

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}

// InvokeRestServiceURLEncoded sends an HTTP request to the specified endpoint with a URL-encoded payload.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *Service) InvokeRestServiceURLEncoded(ctx context.Context,
	method string, endpointURL string, payload url.Values,
	headers map[string]string, opts ...HTTPOption) (int, []byte, error) {
	if headers == nil {
		headers = map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}
	}

	logger := s.Log(ctx).WithField("method", method).WithField("endpoint", endpointURL).WithField("header", headers)

	// Apply options
	config := &httpConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Apply timeout if specified
	if config.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return 0, nil, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	cfg, ok := s.Config().(ConfigurationLogLevel)
	if ok && cfg.LoggingLevelIsDebug() {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		logger.WithField("request", string(reqDump)).Debug("request out")
	}

	//nolint:bodyclose //this is done by util.CloseAndLogOnError()
	resp, err := s.HTTPClient().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer util.CloseAndLogOnError(ctx, resp.Body)

	if ok && cfg.LoggingLevelIsDebug() {
		respDump, _ := httputil.DumpResponse(resp, true)
		s.Log(ctx).WithField("response", string(respDump)).Debug("response in")
	}

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}

// HTTPClient obtains an instrumented http client for making appropriate calls downstream.
func (s *Service) HTTPClient() *http.Client {
	return s.client
}
