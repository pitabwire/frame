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
	"github.com/sony/gobreaker/v2"
)

const (
	defaultMaxResponseBodyLen        = 100 << 20 // 100MB default safety cap
	defaultCircuitBreakerMaxRequests = 3
	defaultCircuitBreakerInterval    = 30 * time.Second
	defaultCircuitBreakerTimeout     = 45 * time.Second
	defaultCircuitBreakerThreshold   = 20
	defaultCircuitBreakerFailureRate = 0.5
)

var ErrResponseTooLarge = errors.New("response body truncated, it exceeds configured limit")

type Manager interface {
	Client(ctx context.Context) *http.Client
	SetClient(ctx context.Context, cl *http.Client)

	Invoke(ctx context.Context,
		method string, endpointURL string, payload any,
		headers http.Header, opts ...HTTPOption) (int, []byte, error)
	InvokeWithURLEncoded(ctx context.Context,
		method string, endpointURL string, payload url.Values,
		headers http.Header, opts ...HTTPOption) (int, []byte, error)
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
}

func (s *InvokeResponse) ToFile(ctx context.Context, writer io.Writer) (int64, error) {
	defer util.CloseAndLogOnError(ctx, s.Body)

	return io.Copy(writer, s.Body)
}

func (s *InvokeResponse) ToContent(ctx context.Context, maxBodyLen int64) ([]byte, error) {
	defer util.CloseAndLogOnError(ctx, s.Body)

	reader := io.Reader(s.Body)

	// Hard cap for large files
	if maxBodyLen > 0 {
		reader = io.LimitReader(s.Body, maxBodyLen+1)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if maxBodyLen > 0 && int64(len(data)) > maxBodyLen {
		return data, ErrResponseTooLarge
	}

	return data, nil
}

type invoker struct {
	breakers    sync.Map // map[string]*gobreaker.CircuitBreaker[*http.Response]
	client      *http.Client
	maxBodyLen  int64
	retryPolicy *RetryPolicy
}

// NewManager creates a new invoker with the provided options.
func NewManager(ctx context.Context, opts ...HTTPOption) Manager {
	httpClient := NewHTTPClient(ctx, opts...)

	cfg := &httpConfig{}
	cfg.process(opts...)

	return &invoker{
		client:      httpClient,
		maxBodyLen:  defaultMaxResponseBodyLen,
		retryPolicy: cfg.retryPolicy,
	}
}

func (s *invoker) breakerFor(key string) *gobreaker.CircuitBreaker[*http.Response] {
	if cb, ok := s.breakers.Load(key); ok {
		fcb, fok := cb.(*gobreaker.CircuitBreaker[*http.Response])
		if !fok {
			return nil
		}
		return fcb
	}

	st := gobreaker.Settings{
		Name:        "http:" + key,
		MaxRequests: defaultCircuitBreakerMaxRequests,
		Interval:    defaultCircuitBreakerInterval,
		Timeout:     defaultCircuitBreakerTimeout,

		ReadyToTrip: func(c gobreaker.Counts) bool {
			if c.Requests < defaultCircuitBreakerThreshold {
				return false
			}
			return float64(c.TotalFailures)/float64(c.Requests) >= defaultCircuitBreakerFailureRate
		},
	}

	//nolint:bodyclose //this is done by consuming party to avoid buffering
	cb := gobreaker.NewCircuitBreaker[*http.Response](st)

	actual, _ := s.breakers.LoadOrStore(key, cb)
	fcb, ok := actual.(*gobreaker.CircuitBreaker[*http.Response])
	if !ok {
		return nil
	}
	return fcb
}

func breakerKey(req *http.Request) string {
	u := *req.URL
	return req.Method + " " + u.Host
}

// Client returns the HTTP client used by the invoker.
func (s *invoker) Client(_ context.Context) *http.Client {
	return s.client
}

// SetClient sets the HTTP client used by the invoker.
func (s *invoker) SetClient(_ context.Context, cl *http.Client) {
	s.client = cl
}

func (s *invoker) execute(
	_ context.Context,
	req *http.Request,
	retry *RetryPolicy,
) (*http.Response, error) {
	key := breakerKey(req)
	cb := s.breakerFor(key)

	return cb.Execute(func() (*http.Response, error) {
		var lastErr error

		for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
			resp, err := s.client.Do(req)
			if err == nil {
				return resp, nil
			}

			lastErr = err

			time.Sleep(retry.Backoff(attempt))
		}

		return nil, lastErr
	})
}

// Invoke convenience method to call a http endpoint and utilize the raw results.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) Invoke(ctx context.Context,
	method string, endpointURL string, payload any,
	headers http.Header, opts ...HTTPOption) (int, []byte, error) {
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

	resp, err := s.InvokeStream(ctx, method, endpointURL, body, headers, opts...)
	if err != nil {
		return 0, nil, err
	}

	bodyRead, err := resp.ToContent(ctx, s.maxBodyLen)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, bodyRead, err
}

// InvokeWithURLEncoded sends an HTTP request to the specified endpoint with a URL-encoded payload.
// Options can be used to configure timeout and other HTTP client behavior.
func (s *invoker) InvokeWithURLEncoded(ctx context.Context,
	method string, endpointURL string, payload url.Values,
	headers http.Header, opts ...HTTPOption) (int, []byte, error) {
	if headers == nil {
		headers = http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded"},
		}
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

	resp, err := s.InvokeStream(ctx, method, endpointURL, strings.NewReader(payload.Encode()), headers, opts...)
	if err != nil {
		return 0, nil, err
	}

	bodyRead, err := resp.ToContent(ctx, s.maxBodyLen)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, bodyRead, err
}

func (s *invoker) InvokeStream(
	ctx context.Context,
	method string,
	endpointURL string,
	body io.Reader,
	headers http.Header,
	opts ...HTTPOption,
) (*InvokeResponse, error) {
	httpCfg := &httpConfig{}
	for _, opt := range opts {
		opt(httpCfg)
	}

	if httpCfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, httpCfg.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, body)
	if err != nil {
		return nil, err
	}
	req.Header = headers

	//nolint:bodyclose //InvokeResponse allows autoclosing after using ToFunctions
	resp, err := s.execute(ctx, req, s.retryPolicy)
	if err != nil {
		return nil, err
	}

	// Caller owns body lifecycle
	return &InvokeResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       resp.Body,
	}, nil
}
