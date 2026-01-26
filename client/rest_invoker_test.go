package client //nolint:testpackage // white-box tests for internal circuit breaker and retry logic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// noRetry returns a retry policy that makes a single attempt with no backoff.
func noRetry() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: 1,
		Backoff:     func(int) time.Duration { return 0 },
	}
}

// fastRetry returns a retry policy with the given attempts and minimal backoff.
func fastRetry(maxAttempts int) *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: maxAttempts,
		Backoff:     func(int) time.Duration { return time.Millisecond },
	}
}

// newTestInvoker creates an invoker with a plain HTTP client (no otelhttp wrapper).
func newTestInvoker(client *http.Client, retry *RetryPolicy) *invoker {
	return &invoker{
		client:      client,
		maxBodyLen:  defaultMaxResponseBodyLen,
		retryPolicy: retry,
	}
}

// loadBreaker pre-loads a circuit breaker with a low threshold so tests can
// trip it without sending 20+ requests.
func loadBreaker(inv *invoker, key string, threshold uint32, cbTimeout time.Duration) {
	st := gobreaker.Settings{
		Name:        "test:" + key,
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     cbTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			if c.Requests < threshold {
				return false
			}
			return float64(c.TotalFailures)/float64(c.Requests) >= 0.5
		},
	}
	cb := gobreaker.NewCircuitBreaker[*http.Response](st)
	inv.breakers.Store(key, cb)
}

// bkey computes the breaker key for a method + server URL, matching breakerKey().
//
//nolint:unparam // method is parameterized to match breakerKey signature
func bkey(method, serverURL string) string {
	u, _ := url.Parse(serverURL)
	return method + " " + u.Host
}

// roundTripFunc adapts a function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// ---------------------------------------------------------------------------
// Unit tests: small functions
// ---------------------------------------------------------------------------

func TestBreakerKey(t *testing.T) {
	tests := []struct {
		method string
		rawURL string
		want   string
	}{
		{"GET", "https://example.com:8080/path?q=1", "GET example.com:8080"},
		{"POST", "http://api.example.com/v1/resource", "POST api.example.com"},
		{"DELETE", "http://localhost:9090/", "DELETE localhost:9090"},
	}
	for _, tt := range tests {
		req, _ := http.NewRequest(tt.method, tt.rawURL, nil)
		if got := breakerKey(req); got != tt.want {
			t.Errorf("breakerKey(%s %s) = %q, want %q", tt.method, tt.rawURL, got, tt.want)
		}
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{301, false},
		{400, false},
		{404, false},
		{500, false},
		{501, false},
		{502, true},
		{503, true},
		{504, true},
		{505, false},
	}
	for _, tt := range tests {
		if got := isRetryableStatus(tt.code); got != tt.want {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestServerError_Error(t *testing.T) {
	err := &serverError{statusCode: 503}
	want := "server error: HTTP 503"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// breakerFor
// ---------------------------------------------------------------------------

func TestBreakerFor_CachesPerKey(t *testing.T) {
	inv := &invoker{}

	cb1 := inv.breakerFor("GET example.com")
	cb2 := inv.breakerFor("GET example.com")
	cb3 := inv.breakerFor("POST example.com")

	if cb1 != cb2 {
		t.Error("same key should return the same breaker instance")
	}
	if cb1 == cb3 {
		t.Error("different keys should return different breaker instances")
	}
}

// ---------------------------------------------------------------------------
// execute: basic behavior
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestExecute_ServerError500_NotRetried(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("fail"))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("expected nil error (serverError unwrapped), got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if c := count.Load(); c != 1 {
		t.Errorf("request count = %d, want 1 (500 is not retried)", c)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fail" {
		t.Errorf("body = %q, want %q", body, "fail")
	}
}

func TestExecute_ServerError501_NotRetried(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotImplemented)
	}
	if c := count.Load(); c != 1 {
		t.Errorf("request count = %d, want 1 (501 is not retried)", c)
	}
}

// ---------------------------------------------------------------------------
// execute: retry on transient status codes (502, 503, 504)
// ---------------------------------------------------------------------------

func TestExecute_RetryableStatus_SucceedsOnRetry(t *testing.T) {
	for _, code := range []int{
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var count atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if count.Add(1) == 1 {
					w.WriteHeader(code)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("recovered"))
			}))
			defer srv.Close()

			inv := newTestInvoker(srv.Client(), fastRetry(3))
			ctx := context.Background()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

			resp, err := inv.execute(ctx, req, inv.retryPolicy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if c := count.Load(); c != 2 {
				t.Errorf("request count = %d, want 2", c)
			}
		})
	}
}

func TestExecute_RetryableStatus_ExhaustedRetries(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unavailable"))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("expected nil error (serverError unwrapped), got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	if c := count.Load(); c != 3 {
		t.Errorf("request count = %d, want 3 (all attempts exhausted)", c)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "unavailable" {
		t.Errorf("body = %q, want %q", body, "unavailable")
	}
}

func TestExecute_4xxNotRetried(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if c := count.Load(); c != 1 {
		t.Errorf("request count = %d, want 1 (4xx is not retried)", c)
	}
}

// ---------------------------------------------------------------------------
// execute: retry on transport errors
// ---------------------------------------------------------------------------

func TestExecute_TransportError_Retried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var rtCount atomic.Int32
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if rtCount.Add(1) == 1 {
				return nil, errors.New("connection refused")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	inv := newTestInvoker(client, fastRetry(3))
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if c := rtCount.Load(); c != 2 {
		t.Errorf("attempt count = %d, want 2", c)
	}
}

func TestExecute_TransportError_AllAttemptsFail(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
	}

	inv := newTestInvoker(client, fastRetry(3))
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unreachable.invalid", nil)

	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
}

// ---------------------------------------------------------------------------
// execute: handles redirect errors gracefully (Client.Do returns resp+err)
// ---------------------------------------------------------------------------

func TestExecute_RedirectError_HandledGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/final" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := srv.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("redirects blocked")
	}

	inv := newTestInvoker(client, noRetry())
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/start", nil)

	// Client.Do returns (resp, err) when CheckRedirect fails.
	// execute must handle this without panicking or leaking.
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if err == nil {
		t.Fatal("expected error for blocked redirect")
	}
	if !strings.Contains(err.Error(), "redirects blocked") {
		t.Errorf("error = %q, want it to contain 'redirects blocked'", err)
	}
}

// ---------------------------------------------------------------------------
// execute: request body reset on retry
// ---------------------------------------------------------------------------

func TestExecute_RequestBodyResetOnRetry(t *testing.T) {
	var receivedBodies []string
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		if count.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway) // retryable
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()

	payload := `{"key":"value"}`
	body := bytes.NewReader([]byte(payload))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, body)

	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(receivedBodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(receivedBodies))
	}
	for i, b := range receivedBodies {
		if b != payload {
			t.Errorf("request %d body = %q, want %q", i+1, b, payload)
		}
	}
}

func TestExecute_NonResettableBody_StopsRetry(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadGateway) // retryable
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()

	// Wrap in a type the stdlib doesn't recognize so GetBody stays nil.
	body := struct{ io.Reader }{strings.NewReader("data")}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, body)

	if req.GetBody != nil {
		t.Fatal("precondition: GetBody should be nil for unrecognized reader type")
	}

	_, err := inv.execute(ctx, req, inv.retryPolicy)
	// Should get an error because the body can't be reset and resp was
	// closed during the retryable-status path.
	if err == nil {
		t.Fatal("expected error when body is non-resettable")
	}
	if c := count.Load(); c != 1 {
		t.Errorf("request count = %d, want 1 (retry should have been aborted)", c)
	}
}

// ---------------------------------------------------------------------------
// execute: context cancellation during backoff
// ---------------------------------------------------------------------------

func TestExecute_ContextCancelledDuringBackoff(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadGateway) // retryable
	}))
	defer srv.Close()

	slowRetry := &RetryPolicy{
		MaxAttempts: 5,
		Backoff:     func(int) time.Duration { return 10 * time.Second },
	}
	inv := newTestInvoker(srv.Client(), slowRetry)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
	if c := count.Load(); c != 1 {
		t.Errorf("expected 1 attempt before context cancel, got %d", c)
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: trips on 5xx server errors
// ---------------------------------------------------------------------------

func TestCircuitBreaker_TripsOn5xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	key := bkey(http.MethodGet, srv.URL)
	loadBreaker(inv, key, 3, time.Minute)

	ctx := context.Background()

	// Send 3 requests that all return 500 → CB records 3 failures.
	for i := range 3 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := inv.execute(ctx, req, inv.retryPolicy)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("request %d: status = %d, want 500", i+1, resp.StatusCode)
		}
	}

	// 4th request should be rejected by the open circuit breaker.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected gobreaker.ErrOpenState, got: %v", err)
	}
}

func TestCircuitBreaker_TripsOnTransportErrors(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
	}

	inv := newTestInvoker(client, noRetry())
	key := bkey(http.MethodGet, "http://unreachable.invalid")
	loadBreaker(inv, key, 3, time.Minute)

	ctx := context.Background()

	// 3 transport errors to trip the breaker.
	for range 3 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unreachable.invalid", nil)
		_, _ = inv.execute(ctx, req, inv.retryPolicy)
	}

	// 4th should be blocked.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unreachable.invalid", nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected gobreaker.ErrOpenState, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: per-host isolation
// ---------------------------------------------------------------------------

func TestCircuitBreaker_PerHostIsolation(t *testing.T) {
	srvFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srvFail.Close()

	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srvOK.Close()

	client := &http.Client{}
	inv := newTestInvoker(client, noRetry())

	keyFail := bkey(http.MethodGet, srvFail.URL)
	keyOK := bkey(http.MethodGet, srvOK.URL)
	loadBreaker(inv, keyFail, 3, time.Minute)
	loadBreaker(inv, keyOK, 3, time.Minute)

	ctx := context.Background()

	// Trip the breaker for srvFail.
	for i := range 3 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srvFail.URL, nil)
		resp, err := inv.execute(ctx, req, inv.retryPolicy)
		if err != nil {
			t.Fatalf("srvFail request %d: unexpected error: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// srvFail should be tripped.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srvFail.URL, nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("srvFail: expected ErrOpenState, got: %v", err)
	}

	// srvOK should still work.
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, srvOK.URL, nil)
	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("srvOK: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("srvOK: status = %d, want 200", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: does not trip below threshold
// ---------------------------------------------------------------------------

func TestCircuitBreaker_DoesNotTripBelowThreshold(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := count.Add(1)
		// Every 3rd request fails (33% failure rate < 50%).
		if n%3 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	key := bkey(http.MethodGet, srv.URL)
	loadBreaker(inv, key, 5, time.Minute)

	ctx := context.Background()

	// Send 9 requests: 6 succeed, 3 fail → 33% failure rate.
	for i := range 9 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := inv.execute(ctx, req, inv.retryPolicy)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// Breaker should still be closed.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("breaker should not have tripped (33%% < 50%%): %v", err)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Circuit breaker: does trip at exactly 50% failure rate
// ---------------------------------------------------------------------------

func TestCircuitBreaker_TripsAtFailureRate(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := count.Add(1)
		// Alternate: odd succeed, even fail → 50% failure rate.
		if n%2 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	key := bkey(http.MethodGet, srv.URL)
	loadBreaker(inv, key, 4, time.Minute)

	ctx := context.Background()

	// Send 4 requests: 2 succeed (n=1,3), 2 fail (n=2,4) → 50%.
	for i := range 4 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := inv.execute(ctx, req, inv.retryPolicy)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// Breaker should be open (50% >= 50%).
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected ErrOpenState at 50%% failure rate, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: recovery after timeout
// ---------------------------------------------------------------------------

func TestCircuitBreaker_RecoverAfterTimeout(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := count.Add(1)
		if n <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	key := bkey(http.MethodGet, srv.URL)
	loadBreaker(inv, key, 3, 100*time.Millisecond)

	ctx := context.Background()

	// Trip the breaker: 3 failures.
	for i := range 3 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := inv.execute(ctx, req, inv.retryPolicy)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// Verify it's open.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected ErrOpenState, got: %v", err)
	}

	// Wait for the CB timeout to expire → half-open.
	time.Sleep(150 * time.Millisecond)

	// Next request should go through (half-open allows 1 request).
	// Server now returns 200 (count=4 > 3).
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := inv.execute(ctx, req, inv.retryPolicy)
	if err != nil {
		t.Fatalf("expected recovery in half-open state, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 after recovery", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: mixed 5xx and retryable status interaction
// ---------------------------------------------------------------------------

func TestCircuitBreaker_RetryableExhaustionCountsAsFailure(t *testing.T) {
	// All requests return 503 (retryable). After retries are exhausted,
	// the CB should record the result as a failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(2))
	key := bkey(http.MethodGet, srv.URL)
	loadBreaker(inv, key, 3, time.Minute)

	ctx := context.Background()

	// Each execute call exhausts 2 attempts → CB records 1 failure.
	for i := range 3 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := inv.execute(ctx, req, inv.retryPolicy)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// Breaker should be open.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := inv.execute(ctx, req, inv.retryPolicy)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected ErrOpenState after exhausted retryable failures, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream: context tied to body lifetime
// ---------------------------------------------------------------------------

func TestInvokeStream_ContextTiedToBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("streaming data"))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()

	resp, err := inv.InvokeStream(ctx, http.MethodGet, srv.URL, nil, nil,
		WithHTTPTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Body should be wrapped with cancelOnCloseBody since timeout was set.
	if _, ok := resp.Body.(*cancelOnCloseBody); !ok {
		t.Errorf("expected cancelOnCloseBody wrapper, got %T", resp.Body)
	}

	// Body should be readable after InvokeStream returns.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(data) != "streaming data" {
		t.Errorf("body = %q, want %q", data, "streaming data")
	}

	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Errorf("closing body: %v", closeErr)
	}
}

func TestInvokeStream_NoTimeoutNoWrapper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()

	resp, err := inv.InvokeStream(ctx, http.MethodGet, srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Without timeout, body should NOT be wrapped.
	if _, ok := resp.Body.(*cancelOnCloseBody); ok {
		t.Error("body should not be wrapped when no timeout is set")
	}
}

// ---------------------------------------------------------------------------
// InvokeStream: seekable body sets GetBody for retries
// ---------------------------------------------------------------------------

func TestInvokeStream_SeekableBodyRetried(t *testing.T) {
	var receivedBodies []string
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		if count.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), fastRetry(3))
	ctx := context.Background()

	payload := `{"test":"data"}`
	resp, err := inv.InvokeStream(ctx, http.MethodPost, srv.URL,
		bytes.NewReader([]byte(payload)),
		http.Header{"Content-Type": {"application/json"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(receivedBodies) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(receivedBodies))
	}
	for i, b := range receivedBodies {
		if b != payload {
			t.Errorf("attempt %d body = %q, want %q", i+1, b, payload)
		}
	}
}

// ---------------------------------------------------------------------------
// InvokeResponse: ToContent
// ---------------------------------------------------------------------------

func TestToContent_TruncatesExactly(t *testing.T) {
	content := strings.Repeat("x", 100)
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(content)),
		maxBodyLen: 50,
	}

	data, err := resp.ToContent(context.Background())
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got: %v", err)
	}
	if len(data) != 50 {
		t.Errorf("len(data) = %d, want exactly 50", len(data))
	}
}

func TestToContent_UnderLimit(t *testing.T) {
	content := "hello"
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(content)),
		maxBodyLen: 100,
	}

	data, err := resp.ToContent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != content {
		t.Errorf("data = %q, want %q", data, content)
	}
}

func TestToContent_NoLimit(t *testing.T) {
	content := strings.Repeat("x", 1000)
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(content)),
	}

	data, err := resp.ToContent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 1000 {
		t.Errorf("len(data) = %d, want 1000", len(data))
	}
}

// ---------------------------------------------------------------------------
// InvokeResponse: Decode
// ---------------------------------------------------------------------------

func TestDecode_Success(t *testing.T) {
	payload := map[string]string{"hello": "world"}
	encoded, _ := json.Marshal(payload)

	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(encoded)),
	}

	var result map[string]string
	if err := resp.Decode(context.Background(), &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["hello"] != "world" {
		t.Errorf("decoded = %v, want map[hello:world]", result)
	}
}

func TestDecode_InvalidJSON(t *testing.T) {
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("not json")),
	}

	var result map[string]string
	if err := resp.Decode(context.Background(), &result); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// Invoke (full path)
// ---------------------------------------------------------------------------

func TestInvoke_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Accept = %q, want application/json", accept)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()

	resp, err := inv.Invoke(ctx, http.MethodGet, srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, err := resp.ToContent(ctx)
	if err != nil {
		t.Fatalf("unexpected error reading body: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestInvoke_WithPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()

	input := map[string]string{"key": "value"}
	resp, err := inv.Invoke(ctx, http.MethodPost, srv.URL, input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var result map[string]string
	err = resp.Decode(ctx, &result)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("result = %v, want map[key:value]", result)
	}
}

func TestInvoke_ServerError_ReturnsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"something broke"}`))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()

	resp, err := inv.Invoke(ctx, http.MethodGet, srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	body, err := resp.ToContent(ctx)
	if err != nil {
		t.Fatalf("unexpected error reading body: %v", err)
	}
	if !strings.Contains(string(body), "something broke") {
		t.Errorf("body = %q, want it to contain error message", body)
	}
}

// ---------------------------------------------------------------------------
// InvokeWithURLEncoded (full path)
// ---------------------------------------------------------------------------

func TestInvokeWithURLEncoded_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		_ = r.ParseForm()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.FormValue("key")))
	}))
	defer srv.Close()

	inv := newTestInvoker(srv.Client(), noRetry())
	ctx := context.Background()

	payload := url.Values{"key": {"value"}}
	resp, err := inv.InvokeWithURLEncoded(ctx, http.MethodPost, srv.URL, payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, err := resp.ToContent(ctx)
	if err != nil {
		t.Fatalf("unexpected error reading body: %v", err)
	}
	if string(body) != "value" {
		t.Errorf("body = %q, want %q", body, "value")
	}
}

// ---------------------------------------------------------------------------
// Client / SetClient
// ---------------------------------------------------------------------------

func TestClient_GetSet(t *testing.T) {
	inv := newTestInvoker(&http.Client{}, noRetry())
	ctx := context.Background()

	original := inv.Client(ctx)
	if original == nil {
		t.Fatal("Client() returned nil")
	}

	replacement := &http.Client{Timeout: 99 * time.Second}
	inv.SetClient(ctx, replacement)

	if got := inv.Client(ctx); got != replacement {
		t.Error("SetClient did not replace the client")
	}
}
