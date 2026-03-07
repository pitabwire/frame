package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pitabwire/util"
)

// resilientTransport is an http.RoundTripper that adds retry logic around an
// inner transport. Requests get automatic retries on transient failures
// (502, 503, 504, and transport errors).
type resilientTransport struct {
	inner       http.RoundTripper
	retryPolicy *RetryPolicy
}

func newResilientTransport(inner http.RoundTripper, retry *RetryPolicy) *resilientTransport {
	if retry == nil {
		retry = &RetryPolicy{
			MaxAttempts: defaultMaxRetryAttempts,
			Backoff: func(attempt int) time.Duration {
				return time.Duration(attempt*attempt) * defaultMinRetryDuration
			},
		}
	}

	return &resilientTransport{
		inner:       inner,
		retryPolicy: retry,
	}
}

func (rt *resilientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	retry := rt.retryPolicy
	var lastResp *http.Response
	var lastErr error

	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		if attempt > 1 {
			if req.GetBody != nil {
				bodyReader, bodyErr := req.GetBody()
				if bodyErr != nil {
					return lastResp, bodyErr
				}
				req.Body = bodyReader
			} else if req.Body != nil {
				return lastResp, errors.New("request body cannot be retried")
			}
		}

		resp, err := rt.inner.RoundTrip(req)
		switch {
		case err != nil:
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			lastResp = nil
			lastErr = err
		case isRetryableStatus(resp.StatusCode) && attempt < retry.MaxAttempts:
			_ = resp.Body.Close()
			lastResp = nil
			lastErr = nil
		default:
			return resp, nil
		}

		t := time.NewTimer(retry.Backoff(attempt))
		select {
		case <-req.Context().Done():
			t.Stop()
			return nil, req.Context().Err()
		case <-t.C:
		}
	}

	return lastResp, lastErr
}

func (rt *resilientTransport) CloseIdleConnections() {
	if closer, ok := rt.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

const defaultMaxResponseBodyLen = 100 << 20 // 100MB default safety cap

var ErrResponseTooLarge = errors.New("response body truncated, it exceeds configured limit")

type Manager interface {
	Client(ctx context.Context) *http.Client
	SetClient(ctx context.Context, cl *http.Client)
	Close()

	Invoke(ctx context.Context,
		method string, endpointURL string, payload any,
		headers http.Header, opts ...HTTPOption) (*InvokeResponse, error)
	InvokeWithURLEncoded(ctx context.Context,
		method string, endpointURL string, payload url.Values,
		headers http.Header, opts ...HTTPOption) (*InvokeResponse, error)
	InvokeStream(
		ctx context.Context,
		method string, endpointURL string,
		body io.Reader,
		headers http.Header,
		opts ...HTTPOption,
	) (*InvokeResponse, error)
}

type InvokeResponse struct {
	StatusCode int
	Headers    http.Header
	Body       io.ReadCloser

	maxBodyLen int64
}

func (s *InvokeResponse) Close() error {
	if s.Body != nil {
		return s.Body.Close()
	}
	return nil
}

func (s *InvokeResponse) ToFile(ctx context.Context, writer io.Writer) (int64, error) {
	defer util.CloseAndLogOnError(ctx, s)

	return io.Copy(writer, s.Body)
}

func (s *InvokeResponse) ToContent(ctx context.Context) ([]byte, error) {
	defer util.CloseAndLogOnError(ctx, s)

	reader := io.Reader(s.Body)

	// Hard cap for large files
	if s.maxBodyLen > 0 {
		reader = io.LimitReader(s.Body, s.maxBodyLen+1)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if s.maxBodyLen > 0 && int64(len(data)) > s.maxBodyLen {
		return data[:s.maxBodyLen], ErrResponseTooLarge
	}

	return data, nil
}

// Decode streams a JSON response directly into v without buffering the entire
// body. The response body is closed after decoding.
func (s *InvokeResponse) Decode(ctx context.Context, v any) error {
	defer util.CloseAndLogOnError(ctx, s.Body)
	return json.NewDecoder(s.Body).Decode(v)
}

type invoker struct {
	client     *http.Client
	maxBodyLen int64
	baseOpts   []HTTPOption
}

// NewManager creates a new invoker with the provided options.
func NewManager(ctx context.Context, opts ...HTTPOption) Manager {
	httpClient, err := newHTTPClient(ctx, opts...)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to initialize HTTP client")
	}

	return &invoker{
		client:     httpClient,
		maxBodyLen: defaultMaxResponseBodyLen,
		baseOpts:   append([]HTTPOption{}, opts...),
	}
}

func (s *invoker) Close() {
	if s.client != nil {
		s.client.CloseIdleConnections()
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

// isRetryableStatus returns true for HTTP status codes that indicate a
// temporary server-side issue worth retrying.
func isRetryableStatus(code int) bool {
	return code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

// Invoke convenience method to call a http endpoint and utilize the raw results.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) Invoke(ctx context.Context,
	method string, endpointURL string, payload any,
	headers http.Header, opts ...HTTPOption) (*InvokeResponse, error) {
	if headers == nil {
		headers = http.Header{
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		}
	}

	var body io.Reader
	if payload != nil {
		postBody, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(postBody)
	}

	resp, err := s.InvokeStream(ctx, method, endpointURL, body, headers, opts...)
	if err != nil {
		return nil, err
	}
	resp.maxBodyLen = s.maxBodyLen

	return resp, err
}

// InvokeWithURLEncoded sends an HTTP request to the specified endpoint with a URL-encoded payload.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) InvokeWithURLEncoded(ctx context.Context,
	method string, endpointURL string, payload url.Values,
	headers http.Header, opts ...HTTPOption) (*InvokeResponse, error) {
	if headers == nil {
		headers = http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded"},
		}
	}

	resp, err := s.InvokeStream(ctx, method, endpointURL, strings.NewReader(payload.Encode()), headers, opts...)
	if err != nil {
		return nil, err
	}

	resp.maxBodyLen = s.maxBodyLen
	return resp, err
}

func (s *invoker) InvokeStream(
	ctx context.Context,
	method string,
	endpointURL string,
	body io.Reader,
	headers http.Header,
	opts ...HTTPOption,
) (*InvokeResponse, error) {
	u, err := util.ValidateHTTPURL(endpointURL)
	if err != nil {
		return nil, err
	}

	req, cancel, httpCfg, err := s.newInvokeRequest(ctx, method, u.String(), body, headers, opts...)
	if err != nil {
		return nil, err
	}

	requestClient, temporaryClient, err := s.clientForInvoke(req.Context(), httpCfg, opts...)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}

	//nolint:bodyclose // response body ownership is transferred to InvokeResponse
	resp, err := s.executeInvokeRequest(requestClient, req, cancel, temporaryClient)
	if err != nil {
		return nil, err
	}

	return newInvokeResponse(resp, cancel, requestClient, temporaryClient), nil
}

func (s *invoker) newInvokeRequest(
	ctx context.Context,
	method string,
	endpointURL string,
	body io.Reader,
	headers http.Header,
	opts ...HTTPOption,
) (*http.Request, context.CancelFunc, *httpConfig, error) {
	httpCfg := &httpConfig{}
	httpCfg.process(ctx, opts...)

	var cancel context.CancelFunc
	if httpCfg.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, httpCfg.timeout)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, body)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, nil, nil, err
	}
	req.Header = headers

	enableBodyRewind(req, body)

	return req, cancel, httpCfg, nil
}

func (s *invoker) clientForInvoke(
	ctx context.Context,
	httpCfg *httpConfig,
	opts ...HTTPOption,
) (*http.Client, bool, error) {
	requestClient := s.client
	temporaryClient := false
	if shouldCreateRequestScopedClient(httpCfg) {
		var err error
		requestClient, err = s.requestScopedClient(ctx, opts...)
		if err != nil {
			return nil, false, err
		}
		temporaryClient = true
	}

	return requestClient, temporaryClient, nil
}

func (s *invoker) executeInvokeRequest(
	requestClient *http.Client,
	req *http.Request,
	cancel context.CancelFunc,
	temporaryClient bool,
) (*http.Response, error) {
	resp, err := requestClient.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if temporaryClient {
			requestClient.CloseIdleConnections()
		}
		if cancel != nil {
			cancel()
		}
		return nil, err
	}

	return resp, nil
}

func newInvokeResponse(
	resp *http.Response,
	cancel context.CancelFunc,
	requestClient *http.Client,
	temporaryClient bool,
) *InvokeResponse {
	var closeFns []func()
	if temporaryClient {
		closeFns = append(closeFns, requestClient.CloseIdleConnections)
	}
	if cancel != nil {
		closeFns = append(closeFns, cancel)
	}

	return &InvokeResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       wrapResponseBody(resp.Body, closeFns...),
	}
}

func (s *invoker) requestScopedClient(ctx context.Context, opts ...HTTPOption) (*http.Client, error) {
	effectiveOpts := append([]HTTPOption{}, s.baseOpts...)
	effectiveOpts = append(effectiveOpts, opts...)
	return newHTTPClient(ctx, effectiveOpts...)
}

func shouldCreateRequestScopedClient(cfg *httpConfig) bool {
	if cfg == nil {
		return false
	}

	return cfg.transport != nil ||
		cfg.jar != nil ||
		cfg.checkRedirect != nil ||
		cfg.idleTimeout > 0 ||
		cfg.enableH2C ||
		cfg.cliCredCfg != nil ||
		cfg.traceRequests ||
		cfg.traceRequestHeaders ||
		cfg.retryPolicyConfigured ||
		cfg.workloadAPIRequested
}

// enableBodyRewind sets GetBody on req so the HTTP client can replay the body
// during redirects or retries. It only applies when the body supports seeking
// and the stdlib hasn't already set GetBody.
func enableBodyRewind(req *http.Request, body io.Reader) {
	if body == nil || req.GetBody != nil {
		return
	}
	seeker, ok := body.(io.ReadSeeker)
	if !ok {
		return
	}
	req.GetBody = func() (io.ReadCloser, error) {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return io.NopCloser(seeker), nil
	}
}

type closeFuncBody struct {
	io.ReadCloser
	closeFns  []func()
	closeOnce sync.Once
}

func wrapResponseBody(body io.ReadCloser, closeFns ...func()) io.ReadCloser {
	if len(closeFns) == 0 {
		return body
	}

	return &closeFuncBody{
		ReadCloser: body,
		closeFns:   closeFns,
	}
}

func (b *closeFuncBody) Close() error {
	err := b.ReadCloser.Close()
	b.closeOnce.Do(func() {
		for _, closeFn := range b.closeFns {
			if closeFn != nil {
				closeFn()
			}
		}
	})
	return err
}
