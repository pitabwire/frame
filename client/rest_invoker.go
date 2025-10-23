package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
)

type HTTPInvoker interface {
	InvokeRestService(ctx context.Context,
		method string, endpointURL string, payload map[string]any,
		headers map[string][]string, opts ...HTTPOption) (int, []byte, error)
	InvokeRestServiceURLEncoded(ctx context.Context,
		method string, endpointURL string, payload url.Values,
		headers map[string]string, opts ...HTTPOption) (int, []byte, error)
}

type invoker struct {
	cfg    config.ConfigurationLogLevel
	client *http.Client
}

// NewInvoker creates a new invoker with the provided options.
func NewInvoker(cfg config.ConfigurationLogLevel, client *http.Client) HTTPInvoker {
	return &invoker{
		cfg:    cfg,
		client: client,
	}
}

// InvokeRestService convenience method to call a http endpoint and utilize the raw results.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) InvokeRestService(ctx context.Context,
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
	httpCfg := &httpConfig{}
	for _, opt := range opts {
		opt(httpCfg)
	}

	// Apply timeout if specified
	if httpCfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, httpCfg.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, body)
	if err != nil {
		return 0, nil, err
	}

	req.Header = headers

	if s.cfg.LoggingLevelIsDebug() {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		util.Log(ctx).WithField("request", string(reqDump)).Debug("request out")
	}

	//nolint:bodyclose //this is done by util.CloseAndLogOnError()
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer util.CloseAndLogOnError(ctx, resp.Body)

	if s.cfg.LoggingLevelIsDebug() {
		respDump, _ := httputil.DumpResponse(resp, true)
		util.Log(ctx).WithField("response", string(respDump)).Debug("response in")
	}

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}

// InvokeRestServiceURLEncoded sends an HTTP request to the specified endpoint with a URL-encoded payload.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) InvokeRestServiceURLEncoded(ctx context.Context,
	method string, endpointURL string, payload url.Values,
	headers map[string]string, opts ...HTTPOption) (int, []byte, error) {
	if headers == nil {
		headers = map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}
	}

	logger := util.Log(ctx).WithField("method", method).WithField("endpoint", endpointURL).WithField("header", headers)

	// Apply options
	httpCfg := &httpConfig{}
	for _, opt := range opts {
		opt(httpCfg)
	}

	// Apply timeout if specified
	if httpCfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, httpCfg.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return 0, nil, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	if s.cfg.LoggingLevelIsDebug() {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		logger.WithField("request", string(reqDump)).Debug("request out")
	}

	//nolint:bodyclose //this is done by util.CloseAndLogOnError()
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer util.CloseAndLogOnError(ctx, resp.Body)

	if s.cfg.LoggingLevelIsDebug() {
		respDump, _ := httputil.DumpResponse(resp, true)
		util.Log(ctx).WithField("response", string(respDump)).Debug("response in")
	}

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}
