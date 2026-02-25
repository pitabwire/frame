package ratelimiter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/ratelimiter"
	"github.com/pitabwire/frame/security"
)

func TestDefaultWindowConfig(t *testing.T) {
	cfg := ratelimiter.DefaultWindowConfig()
	require.NotNil(t, cfg)
	assert.Greater(t, cfg.WindowDuration, time.Duration(0))
	assert.Greater(t, cfg.MaxPerWindow, 0)
	assert.NotEmpty(t, cfg.KeyPrefix)
}

func TestIPRateLimiterAllow(t *testing.T) {
	raw := cache.NewInMemoryCache()
	t.Cleanup(func() { _ = raw.Close() })

	cfg := &ratelimiter.WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   2,
		KeyPrefix:      "test:ip",
		FailOpen:       false,
	}

	limiter := ratelimiter.NewIPRateLimiter(raw, cfg)
	t.Cleanup(func() { _ = limiter.Close() })

	ctx := context.Background()
	assert.True(t, limiter.Allow(ctx, "127.0.0.1"))
	assert.True(t, limiter.Allow(ctx, "127.0.0.1"))
	assert.False(t, limiter.Allow(ctx, "127.0.0.1"))
}

func TestRateLimitMiddleware(t *testing.T) {
	raw := cache.NewInMemoryCache()
	t.Cleanup(func() { _ = raw.Close() })

	cfg := &ratelimiter.WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   1,
		KeyPrefix:      "test:ip:mw",
		FailOpen:       false,
	}

	limiter := ratelimiter.NewIPRateLimiter(raw, cfg)
	t.Cleanup(func() { _ = limiter.Close() })

	mw := ratelimiter.RateLimitMiddleware(limiter)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	assert.Equal(t, http.StatusOK, rr1.Code)

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
	assert.Equal(t, "0", rr2.Header().Get("X-RateLimit-Remaining"))
}

func TestUserRateLimitMiddlewarePrefersUser(t *testing.T) {
	raw := cache.NewInMemoryCache()
	t.Cleanup(func() { _ = raw.Close() })

	cfg := &ratelimiter.WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   1,
		FailOpen:       false,
	}

	userLimiter := ratelimiter.NewUserRateLimiter(raw, cfg)
	ipLimiter := ratelimiter.NewIPRateLimiter(raw, cfg)
	t.Cleanup(func() { _ = userLimiter.Close() })
	t.Cleanup(func() { _ = ipLimiter.Close() })

	mw := ratelimiter.UserRateLimitMiddleware(userLimiter, ipLimiter)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	claims := &security.AuthenticationClaims{}
	claims.Subject = "user-1"
	req = req.WithContext(claims.ClaimsToContext(req.Context()))

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	assert.Equal(t, http.StatusOK, rr1.Code)
	assert.Equal(t, "user", rr1.Header().Get("X-RateLimit-Scope"))

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
	assert.Equal(t, "user", rr2.Header().Get("X-RateLimit-Scope"))
}

func TestWindowLimiterAllow(t *testing.T) {
	raw := cache.NewInMemoryCache()
	t.Cleanup(func() { _ = raw.Close() })

	wl := ratelimiter.NewWindowLimiter(raw, &ratelimiter.WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   2,
		KeyPrefix:      "test",
		FailOpen:       false,
	})

	ctx := context.Background()
	assert.True(t, wl.Allow(ctx, "tenant-1"))
	assert.True(t, wl.Allow(ctx, "tenant-1"))
	assert.False(t, wl.Allow(ctx, "tenant-1"))
	assert.True(t, wl.Allow(ctx, "tenant-2"))
}
