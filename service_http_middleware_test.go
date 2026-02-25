package frame_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/ratelimiter"
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
	defer svc.Stop(ctx)

	err := svc.Run(ctx, "")
	require.NoError(t, err)

	ts := tsGetter()
	require.NotNil(t, ts)

	resp, reqErr := ts.Client().Get(ts.URL + "/")
	require.NoError(t, reqErr)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ABH", trace)
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
	defer svc.Stop(ctx)

	err := svc.Run(ctx, "")
	require.NoError(t, err)

	ts := tsGetter()
	require.NotNil(t, ts)

	resp, reqErr := ts.Client().Get(ts.URL + "/")
	require.NoError(t, reqErr)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestWithHTTPMiddlewareRateLimiter(t *testing.T) {
	cfg := &ratelimiter.RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         1,
		CleanupInterval:   time.Minute,
		EntryTTL:          time.Minute,
		MaxEntries:        100,
	}
	ipLimiter := ratelimiter.NewIPRateLimiter(cfg)
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
	defer svc.Stop(ctx)

	err := svc.Run(ctx, "")
	require.NoError(t, err)

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
}
