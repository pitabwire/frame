package client

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pitabwire/util"
	"github.com/sony/gobreaker/v2"
)

// resilientTransport is an http.RoundTripper that adds circuit breaker and
// retry logic around an inner transport. Every request that flows through
// this transport gets automatic per-host circuit breaking and retries on
// transient failures (502, 503, 504, transport errors).
type resilientTransport struct {
	inner       http.RoundTripper
	breakers    sync.Map // map[string]*gobreaker.CircuitBreaker[*http.Response]
	retryPolicy *RetryPolicy

	breakerMu    sync.Mutex
	breakerOrder *list.List
	breakerIndex map[string]*list.Element
	maxBreakers  int
	breakerTTL   time.Duration
}

func newResilientTransport(inner http.RoundTripper, retry *RetryPolicy) *resilientTransport {
	return &resilientTransport{
		inner:        inner,
		retryPolicy:  retry,
		breakerOrder: list.New(),
		breakerIndex: map[string]*list.Element{},
		maxBreakers:  defaultCircuitBreakerMaxEntries,
		breakerTTL:   defaultCircuitBreakerIdleTTL,
	}
}

func (rt *resilientTransport) breakerFor(key string) *gobreaker.CircuitBreaker[*http.Response] {
	if cb, ok := rt.breakers.Load(key); ok {
		rt.touchBreaker(key)
		//nolint:errcheck // only *gobreaker.CircuitBreaker[*http.Response] is stored
		return cb.(*gobreaker.CircuitBreaker[*http.Response])
	}

	rt.breakerMu.Lock()
	defer rt.breakerMu.Unlock()

	rt.ensureBreakerStateLocked()

	if cb, ok := rt.breakers.Load(key); ok {
		rt.touchBreakerLocked(key, time.Now())
		//nolint:errcheck // only *gobreaker.CircuitBreaker[*http.Response] is stored
		return cb.(*gobreaker.CircuitBreaker[*http.Response])
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

	rt.breakers.Store(key, cb)
	rt.touchBreakerLocked(key, time.Now())
	rt.evictBreakersLocked()

	return cb
}

//nolint:gocognit // retry loop with circuit breaker is inherently complex
func (rt *resilientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := breakerKey(req)
	cb := rt.breakerFor(key)
	retry := rt.retryPolicy

	resp, err := cb.Execute(func() (*http.Response, error) {
		var lastResp *http.Response
		var lastErr error

		for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
			// Reset the request body before each retry attempt.
			if attempt > 1 {
				if req.GetBody != nil {
					bodyReader, bErr := req.GetBody()
					if bErr != nil {
						return lastResp, lastErr
					}
					req.Body = bodyReader
				} else if req.Body != nil {
					// Body is non-nil and non-resettable; cannot retry.
					return lastResp, lastErr
				}
			}

			resp, doErr := rt.inner.RoundTrip(req)
			switch {
			case doErr != nil:
				// Always close body when RoundTrip returns both a response and an error.
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
				lastErr = doErr
				lastResp = nil
			case isRetryableStatus(resp.StatusCode) && attempt < retry.MaxAttempts:
				// Transient server error — close body and retry.
				_ = resp.Body.Close()
				lastErr = &serverError{statusCode: resp.StatusCode}
				lastResp = nil
			case resp.StatusCode >= http.StatusInternalServerError:
				// Non-retryable 5xx or final attempt: signal CB failure.
				return resp, &serverError{statusCode: resp.StatusCode}
			default:
				return resp, nil
			}

			// Respect context cancellation during backoff.
			t := time.NewTimer(retry.Backoff(attempt))
			select {
			case <-req.Context().Done():
				t.Stop()
				return nil, req.Context().Err()
			case <-t.C:
			}
		}

		return lastResp, lastErr
	})

	// Unwrap serverError so callers can still read the response body.
	// Only unwrap when resp is non-nil; a nil resp with a nil error would
	// cause a panic in the caller.
	var sErr *serverError
	if resp != nil && errors.As(err, &sErr) {
		return resp, nil
	}

	return resp, err
}

const (
	defaultMaxResponseBodyLen        = 100 << 20 // 100MB default safety cap
	defaultCircuitBreakerMaxRequests = 3
	defaultCircuitBreakerInterval    = 30 * time.Second
	defaultCircuitBreakerTimeout     = 45 * time.Second
	defaultCircuitBreakerThreshold   = 20
	defaultCircuitBreakerFailureRate = 0.5
	defaultCircuitBreakerMaxEntries  = 1024
	defaultCircuitBreakerIdleTTL     = 15 * time.Minute
)

var ErrResponseTooLarge = errors.New("response body truncated, it exceeds configured limit")

// serverError wraps a 5xx response so the circuit breaker records it as a
// failure, while still allowing callers to read the response body.
type serverError struct {
	statusCode int
}

func (e *serverError) Error() string {
	return fmt.Sprintf("server error: HTTP %d", e.statusCode)
}

type breakerMeta struct {
	key      string
	lastUsed time.Time
}

func (rt *resilientTransport) touchBreaker(key string) {
	rt.breakerMu.Lock()
	defer rt.breakerMu.Unlock()

	rt.ensureBreakerStateLocked()
	rt.touchBreakerLocked(key, time.Now())
	rt.evictBreakersLocked()
}

func (rt *resilientTransport) touchBreakerLocked(key string, now time.Time) {
	if elem, ok := rt.breakerIndex[key]; ok {
		meta, _ := elem.Value.(*breakerMeta)
		if meta == nil {
			meta = &breakerMeta{key: key}
			elem.Value = meta
		}
		meta.lastUsed = now
		rt.breakerOrder.MoveToFront(elem)
		return
	}

	meta := &breakerMeta{
		key:      key,
		lastUsed: now,
	}
	rt.breakerIndex[key] = rt.breakerOrder.PushFront(meta)
}

func (rt *resilientTransport) evictBreakersLocked() {
	if rt.breakerOrder == nil || len(rt.breakerIndex) == 0 {
		return
	}

	now := time.Now()

	// Remove stale entries first.
	if rt.breakerTTL > 0 {
		for elem := rt.breakerOrder.Back(); elem != nil; {
			prev := elem.Prev()
			meta, _ := elem.Value.(*breakerMeta)
			if meta == nil || now.Sub(meta.lastUsed) > rt.breakerTTL {
				rt.removeBreakerLocked(elem)
			}
			elem = prev
		}
	}

	// Enforce hard bound.
	for rt.maxBreakers > 0 && len(rt.breakerIndex) > rt.maxBreakers {
		elem := rt.breakerOrder.Back()
		if elem == nil {
			break
		}
		rt.removeBreakerLocked(elem)
	}
}

func (rt *resilientTransport) removeBreakerLocked(elem *list.Element) {
	meta, _ := elem.Value.(*breakerMeta)
	if meta != nil {
		delete(rt.breakerIndex, meta.key)
		rt.breakers.Delete(meta.key)
	}

	rt.breakerOrder.Remove(elem)
}

func (rt *resilientTransport) ensureBreakerStateLocked() {
	if rt.breakerOrder == nil {
		rt.breakerOrder = list.New()
	}
	if rt.breakerIndex == nil {
		rt.breakerIndex = map[string]*list.Element{}
	}
}

type Manager interface {
	Client(ctx context.Context) *http.Client
	SetClient(ctx context.Context, cl *http.Client)

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

// cancelOnCloseBody wraps an io.ReadCloser and calls a cancel function when
// the body is closed. This ties a context's lifetime to the body lifetime,
// preventing the context from being cancelled before the caller finishes
// reading the stream.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
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
}

// NewManager creates a new invoker with the provided options.
func NewManager(ctx context.Context, opts ...HTTPOption) Manager {
	httpClient := NewHTTPClient(ctx, opts...)

	return &invoker{
		client:     httpClient,
		maxBodyLen: defaultMaxResponseBodyLen,
	}
}

func breakerKey(req *http.Request) string {
	return req.Method + " " + req.URL.Host
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

	httpCfg := &httpConfig{}
	for _, opt := range opts {
		opt(httpCfg)
	}

	var cancel context.CancelFunc
	if httpCfg.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, httpCfg.timeout)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	req.Header = headers

	enableBodyRewind(req, body)

	resp, err := s.client.Do(req)
	if err != nil {
		// client.Do may return (resp, err) on redirect errors; close body to avoid leak.
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if cancel != nil {
			cancel()
		}
		return nil, err
	}

	// Tie context lifetime to body lifetime so the caller can read the
	// stream without the context being cancelled on function return.
	respBody := resp.Body
	if cancel != nil {
		respBody = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	}

	// Caller owns body lifecycle
	return &InvokeResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
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
