package ratelimiter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/ratelimiter"
	"github.com/pitabwire/frame/security"
)

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := ratelimiter.DefaultRateLimiterConfig()
	require.NotNil(t, cfg)
	assert.Greater(t, cfg.RequestsPerSecond, 0)
	assert.Greater(t, cfg.BurstSize, 0)
	assert.Greater(t, cfg.CleanupInterval, time.Duration(0))
	assert.Greater(t, cfg.EntryTTL, time.Duration(0))
	assert.Greater(t, cfg.MaxEntries, 0)
}

func TestIPRateLimiterAllow(t *testing.T) {
	cfg := &ratelimiter.RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         2,
		CleanupInterval:   50 * time.Millisecond,
		EntryTTL:          200 * time.Millisecond,
		MaxEntries:        100,
	}

	limiter := ratelimiter.NewIPRateLimiter(cfg)
	t.Cleanup(func() { _ = limiter.Close() })

	assert.True(t, limiter.Allow("127.0.0.1"))
	assert.True(t, limiter.Allow("127.0.0.1"))
	assert.False(t, limiter.Allow("127.0.0.1"))
}

func TestRateLimitMiddleware(t *testing.T) {
	cfg := &ratelimiter.RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         1,
		CleanupInterval:   time.Minute,
		EntryTTL:          time.Minute,
		MaxEntries:        100,
	}

	limiter := ratelimiter.NewIPRateLimiter(cfg)
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
	cfg := &ratelimiter.RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         1,
		CleanupInterval:   time.Minute,
		EntryTTL:          time.Minute,
		MaxEntries:        100,
	}

	userLimiter := ratelimiter.NewUserRateLimiter(cfg)
	ipLimiter := ratelimiter.NewIPRateLimiter(cfg)
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

func TestKeyedLimiterBoundedEntries(t *testing.T) {
	cfg := &ratelimiter.RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         1,
		CleanupInterval:   time.Minute,
		EntryTTL:          time.Minute,
		MaxEntries:        3,
	}

	kl := ratelimiter.NewKeyedLimiter(cfg)
	t.Cleanup(func() { _ = kl.Close() })

	for _, key := range []string{"a", "b", "c", "d", "e"} {
		_ = kl.Allow(key)
	}

	assert.LessOrEqual(t, kl.Len(), 3)
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

func TestKeyedLimiterConcurrent(t *testing.T) {
	cfg := &ratelimiter.RateLimiterConfig{
		RequestsPerSecond: 1000,
		BurstSize:         100,
		CleanupInterval:   time.Minute,
		EntryTTL:          time.Minute,
		MaxEntries:        100,
	}

	kl := ratelimiter.NewKeyedLimiter(cfg)
	t.Cleanup(func() { _ = kl.Close() })

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = kl.Allow("shared")
			}
		}()
	}
	wg.Wait()
}
