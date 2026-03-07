package client //nolint:testpackage // white-box tests for internal transport behavior

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

	"github.com/stretchr/testify/suite"
)

type InvokerSuite struct {
	suite.Suite
}

func TestInvokerSuite(t *testing.T) {
	suite.Run(t, new(InvokerSuite))
}

func noRetry() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: 1,
		Backoff:     func(int) time.Duration { return 0 },
	}
}

func fastRetry() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: 3,
		Backoff:     func(int) time.Duration { return time.Millisecond },
	}
}

func newTestTransport(inner http.RoundTripper, retry *RetryPolicy) *resilientTransport {
	return newResilientTransport(inner, retry)
}

func newTestInvoker(inner http.RoundTripper, retry *RetryPolicy) *invoker {
	rt := newTestTransport(inner, retry)
	return &invoker{
		client:     &http.Client{Transport: rt},
		maxBodyLen: defaultMaxResponseBodyLen,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (s *InvokerSuite) TestIsRetryableStatus() {
	testCases := []struct {
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

	for _, tc := range testCases {
		s.Equal(tc.want, isRetryableStatus(tc.code))
	}
}

func (s *InvokerSuite) TestResilientTransportSuccess() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, noRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	s.Equal("ok", string(body))
}

func (s *InvokerSuite) TestResilientTransportServerError500NotRetried() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("fail"))
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, fastRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusInternalServerError, resp.StatusCode)
	s.Equal(int32(1), count.Load())

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	s.Equal("fail", string(body))
}

func (s *InvokerSuite) TestResilientTransportServerError501NotRetried() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, fastRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusNotImplemented, resp.StatusCode)
	s.Equal(int32(1), count.Load())
}

func (s *InvokerSuite) TestResilientTransportRetryableStatusSucceedsOnRetry() {
	for _, code := range []int{
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		statusCode := code
		s.Run(http.StatusText(statusCode), func() {
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if count.Add(1) == 1 {
					w.WriteHeader(statusCode)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("recovered"))
			}))
			defer server.Close()

			rt := newTestTransport(server.Client().Transport, fastRetry())
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			s.Require().NoError(err)

			resp, err := rt.RoundTrip(req)
			s.Require().NoError(err)
			defer resp.Body.Close()

			s.Equal(http.StatusOK, resp.StatusCode)
			s.Equal(int32(2), count.Load())
		})
	}
}

func (s *InvokerSuite) TestResilientTransportRetryableStatusExhaustsRetries() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, fastRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusServiceUnavailable, resp.StatusCode)
	s.Equal(int32(3), count.Load())

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	s.Equal("unavailable", string(body))
}

func (s *InvokerSuite) TestResilientTransport4xxNotRetried() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, fastRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	s.Equal(int32(1), count.Load())
}

func (s *InvokerSuite) TestResilientTransportTransportErrorRetried() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var count atomic.Int32
	inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if count.Add(1) == 1 {
			return nil, errors.New("connection refused")
		}
		return http.DefaultTransport.RoundTrip(req)
	})

	rt := newTestTransport(inner, fastRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)
	s.Equal(int32(2), count.Load())
}

func (s *InvokerSuite) TestResilientTransportTransportErrorAllAttemptsFail() {
	inner := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})

	rt := newTestTransport(inner, fastRetry())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://unreachable.invalid", nil)
	s.Require().NoError(err)

	_, err = rt.RoundTrip(req)
	s.Require().Error(err)
}

func (s *InvokerSuite) TestResilientTransportRequestBodyResetOnRetry() {
	var receivedBodies []string
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.T().Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBodies = append(receivedBodies, string(body))
		if count.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, fastRetry())
	payload := `{"key":"value"}`
	body := bytes.NewReader([]byte(payload))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, body)
	s.Require().NoError(err)
	req.GetBody = func() (io.ReadCloser, error) {
		_, seekErr := body.Seek(0, io.SeekStart)
		if seekErr != nil {
			return nil, seekErr
		}
		return io.NopCloser(body), nil
	}

	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)
	s.Len(receivedBodies, 2)
	for _, received := range receivedBodies {
		s.Equal(payload, received)
	}
}

func (s *InvokerSuite) TestResilientTransportNonResettableBodyStopsRetry() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	rt := newTestTransport(server.Client().Transport, fastRetry())
	body := struct{ io.Reader }{Reader: strings.NewReader("data")}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, io.NopCloser(body))
	s.Require().NoError(err)
	s.Nil(req.GetBody)

	_, err = rt.RoundTrip(req)
	s.Require().Error(err)
	s.Equal(int32(1), count.Load())
}

func (s *InvokerSuite) TestResilientTransportContextCancelledDuringBackoff() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	slowRetry := &RetryPolicy{
		MaxAttempts: 5,
		Backoff:     func(int) time.Duration { return 10 * time.Second },
	}
	rt := newTestTransport(server.Client().Transport, slowRetry)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	s.Require().NoError(err)

	_, err = rt.RoundTrip(req)
	s.Require().ErrorIs(err, context.DeadlineExceeded)
	s.Equal(int32(1), count.Load())
}

func (s *InvokerSuite) TestInvokeStreamContextTiedToBody() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("streaming data"))
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	resp, err := inv.InvokeStream(context.Background(), http.MethodGet, server.URL, nil, nil,
		WithHTTPTimeout(5*time.Second))
	s.Require().NoError(err)

	_, ok := resp.Body.(*closeFuncBody)
	s.True(ok)

	data, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	s.Equal("streaming data", string(data))
	s.NoError(resp.Body.Close())
}

func (s *InvokerSuite) TestInvokeStreamNoTimeoutNoWrapper() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	resp, err := inv.InvokeStream(context.Background(), http.MethodGet, server.URL, nil, nil)
	s.Require().NoError(err)
	defer resp.Body.Close()

	_, ok := resp.Body.(*closeFuncBody)
	s.False(ok)
}

func (s *InvokerSuite) TestInvokeStreamSeekableBodyRetried() {
	var receivedBodies []string
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.T().Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBodies = append(receivedBodies, string(body))
		if count.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, fastRetry())
	payload := `{"test":"data"}`
	resp, err := inv.InvokeStream(context.Background(), http.MethodPost, server.URL,
		bytes.NewReader([]byte(payload)),
		http.Header{"Content-Type": {"application/json"}})
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)
	s.Len(receivedBodies, 2)
	for _, received := range receivedBodies {
		s.Equal(payload, received)
	}
}

func (s *InvokerSuite) TestInvokeStreamAppliesPerRequestWorkloadAPIOptions() {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	resp, err := inv.InvokeStream(
		context.Background(),
		http.MethodGet,
		server.URL,
		nil,
		nil,
		WithHTTPWorkloadAPITargetID("not-a-valid-spiffe-id"),
	)

	s.Nil(resp)
	s.Require().Error(err)
	s.Equal(int32(0), count.Load())
}

func (s *InvokerSuite) TestToContentTruncatesExactly() {
	content := strings.Repeat("x", 100)
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(content)),
		maxBodyLen: 50,
	}

	data, err := resp.ToContent(context.Background())
	s.Require().ErrorIs(err, ErrResponseTooLarge)
	s.Len(data, 50)
}

func (s *InvokerSuite) TestToContentUnderLimit() {
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("hello")),
		maxBodyLen: 100,
	}

	data, err := resp.ToContent(context.Background())
	s.Require().NoError(err)
	s.Equal("hello", string(data))
}

func (s *InvokerSuite) TestToContentNoLimit() {
	content := strings.Repeat("x", 1000)
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(content)),
	}

	data, err := resp.ToContent(context.Background())
	s.Require().NoError(err)
	s.Len(data, 1000)
}

func (s *InvokerSuite) TestDecodeSuccess() {
	payload := map[string]string{"hello": "world"}
	encoded, err := json.Marshal(payload)
	s.Require().NoError(err)

	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(encoded)),
	}

	var result map[string]string
	s.Require().NoError(resp.Decode(context.Background(), &result))
	s.Equal("world", result["hello"])
}

func (s *InvokerSuite) TestDecodeInvalidJSON() {
	resp := &InvokeResponse{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("not json")),
	}

	var result map[string]string
	s.Require().Error(resp.Decode(context.Background(), &result))
}

func (s *InvokerSuite) TestInvokeSuccess() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("application/json", r.Header.Get("Content-Type"))
		s.Equal("application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	resp, err := inv.Invoke(context.Background(), http.MethodGet, server.URL, nil, nil)
	s.Require().NoError(err)

	s.Equal(http.StatusOK, resp.StatusCode)
	body, err := resp.ToContent(context.Background())
	s.Require().NoError(err)
	s.NotEmpty(body)
}

func (s *InvokerSuite) TestInvokeWithPayload() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.T().Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	input := map[string]string{"key": "value"}
	resp, err := inv.Invoke(context.Background(), http.MethodPost, server.URL, input, nil)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var result map[string]string
	s.Require().NoError(resp.Decode(context.Background(), &result))
	s.Equal("value", result["key"])
}

func (s *InvokerSuite) TestInvokeServerErrorReturnsStatusAndBody() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"something broke"}`))
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	resp, err := inv.Invoke(context.Background(), http.MethodGet, server.URL, nil, nil)
	s.Require().NoError(err)
	s.Equal(http.StatusInternalServerError, resp.StatusCode)

	body, err := resp.ToContent(context.Background())
	s.Require().NoError(err)
	s.Contains(string(body), "something broke")
}

func (s *InvokerSuite) TestInvokeWithURLEncodedSuccess() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		if err := r.ParseForm(); err != nil {
			s.T().Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.FormValue("key")))
	}))
	defer server.Close()

	inv := newTestInvoker(server.Client().Transport, noRetry())
	payload := url.Values{"key": {"value"}}
	resp, err := inv.InvokeWithURLEncoded(context.Background(), http.MethodPost, server.URL, payload, nil)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	body, err := resp.ToContent(context.Background())
	s.Require().NoError(err)
	s.Equal("value", string(body))
}

func (s *InvokerSuite) TestClientGetSet() {
	inv := newTestInvoker(http.DefaultTransport, noRetry())

	original := inv.Client(context.Background())
	s.NotNil(original)

	replacement := &http.Client{Timeout: 99 * time.Second}
	inv.SetClient(context.Background(), replacement)
	s.Same(replacement, inv.Client(context.Background()))
}
