package frame_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/cache"
	"github.com/pitabwire/frame/v2/frametests"
	"github.com/pitabwire/frame/v2/ratelimiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithHTTPMiddlewareOrder(t *testing.T) {
	trace := ""

	mwA := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trace += "A"
			next.ServeHTTP(w, r)
		})
	}
	mwB := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trace += "B"
			next.ServeHTTP(w, r)
		})
	}

	hOpt, tsGetter := frametests.WithHTTPTestDriver()
	ctx, svc := frame.NewService(
		frame.WithName("test"),
		hOpt,
		frame.WithHTTPMiddleware(mwA, mwB),
		frame.WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			trace += "H"
			w.WriteHeader(http.StatusOK)
		})),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx, "")
	}()

	time.Sleep(100 * time.Millisecond)

	ts := tsGetter()
	require.NotNil(t, ts)

	resp, reqErr := ts.Client().Get(ts.URL + "/")
	require.NoError(t, reqErr)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ABH", trace)

	svc.Stop(ctx)
	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
}

func TestWithHTTPMiddlewareCanShortCircuit(t *testing.T) {
	hOpt, tsGetter := frametests.WithHTTPTestDriver()
	ctx, svc := frame.NewService(
		frame.WithName("test"),
		hOpt,
		frame.WithHTTPMiddleware(func(_ http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			})
		}),
		frame.WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx, "")
	}()

	time.Sleep(100 * time.Millisecond)

	ts := tsGetter()
	require.NotNil(t, ts)

	resp, reqErr := ts.Client().Get(ts.URL + "/")
	require.NoError(t, reqErr)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	svc.Stop(ctx)
	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
}

func TestWithHTTPMiddlewareRateLimiter(t *testing.T) {
	backend := cache.NewInMemoryCache()
	defer func() { _ = backend.Close() }()

	cfg := &ratelimiter.WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   1,
		FailOpen:       false,
	}
	ipLimiter, err := ratelimiter.NewIPRateLimiter(backend, cfg)
	require.NoError(t, err)
	defer func() { _ = ipLimiter.Close() }()

	hOpt, tsGetter := frametests.WithHTTPTestDriver()
	ctx, svc := frame.NewService(
		frame.WithName("test"),
		hOpt,
		frame.WithHTTPMiddleware(ratelimiter.RateLimitMiddleware(ipLimiter)),
		frame.WithHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx, "")
	}()

	time.Sleep(100 * time.Millisecond)

	ts := tsGetter()
	require.NotNil(t, ts)

	resp1, reqErr := ts.Client().Get(ts.URL + "/")
	require.NoError(t, reqErr)
	defer resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	resp2, reqErr2 := ts.Client().Get(ts.URL + "/")
	require.NoError(t, reqErr2)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp2.StatusCode)

	svc.Stop(ctx)
	err = <-errCh
	require.ErrorIs(t, err, context.Canceled)
}
