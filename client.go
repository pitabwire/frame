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
)

// HTTPOption configures HTTP client behavior.
type HTTPOption func(*httpConfig)

// httpConfig holds HTTP client configuration.
type httpConfig struct {
	timeout time.Duration
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) HTTPOption {
	return func(c *httpConfig) {
		c.timeout = timeout
	}
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
