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

type Manager interface {
	Client(ctx context.Context) *http.Client
	SetClient(ctx context.Context, cl *http.Client)

	Invoke(ctx context.Context,
		method string, endpointURL string, payload map[string]any,
		headers map[string][]string, opts ...HTTPOption) (int, []byte, error)
	InvokeWithURLEncoded(ctx context.Context,
		method string, endpointURL string, payload url.Values,
		headers map[string]string, opts ...HTTPOption) (int, []byte, error)
}

type invoker struct {
	cfg    config.ConfigurationTraceRequests
	client *http.Client
}

// NewManager creates a new invoker with the provided options.
func NewManager(cfg config.ConfigurationTraceRequests, opts ...HTTPOption) Manager {
	return &invoker{
		cfg:    cfg,
		client: NewHTTPClient(opts...),
	}
}

// Client returns the HTTP client used by the invoker.
func (s *invoker) Client(_ context.Context) *http.Client {
	return s.client
}

// SetClient sets the HTTP client used by the invoker.
func (s *invoker) SetClient(_ context.Context, cl *http.Client) {
	s.client = cl
}

// Invoke convenience method to call a http endpoint and utilize the raw results.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) Invoke(ctx context.Context,
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

	//nolint:bodyclose //this is done by util.CloseAndLogOnError()
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer util.CloseAndLogOnError(ctx, resp.Body)

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}

// InvokeWithURLEncoded sends an HTTP request to the specified endpoint with a URL-encoded payload.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) InvokeWithURLEncoded(ctx context.Context,
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

	if s.cfg.TraceReq() {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		logger.WithField("request", string(reqDump)).Debug("request out")
	}

	//nolint:bodyclose //this is done by util.CloseAndLogOnError()
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer util.CloseAndLogOnError(ctx, resp.Body)

	if s.cfg.TraceReq() {
		respDump, _ := httputil.DumpResponse(resp, true)
		util.Log(ctx).WithField("response", string(respDump)).Debug("response in")
	}

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}
