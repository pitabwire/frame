package ratelimiter

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/security"
)

const (
	defaultIPPrefix   = "ratelimit:ip"
	defaultUserPrefix = "ratelimit:user"
)

// IPRateLimiter applies cache-backed per-IP window limits.
type IPRateLimiter struct {
	limiter  *WindowLimiter
	config   WindowConfig
	backend  cache.RawCache
	ownsBack bool
}

// NewIPRateLimiter creates a new cache-backed IP rate limiter.
// If raw is nil, an in-memory frame cache is created.
func NewIPRateLimiter(raw cache.RawCache, config *WindowConfig) *IPRateLimiter {
	backend := raw
	owns := false
	if backend == nil {
		backend = cache.NewInMemoryCache()
		owns = true
	}

	cfg := normalizeWindowConfig(config)
	if cfg.KeyPrefix == defaultWindowPrefix {
		cfg.KeyPrefix = defaultIPPrefix
	}

	return &IPRateLimiter{
		limiter:  NewWindowLimiter(backend, &cfg),
		config:   cfg,
		backend:  backend,
		ownsBack: owns,
	}
}

// Allow checks whether a request from the given IP should be allowed.
func (rl *IPRateLimiter) Allow(ctx context.Context, ip string) bool {
	if rl == nil || rl.limiter == nil {
		return true
	}
	return rl.limiter.Allow(ctx, ip)
}

// Close releases owned resources.
func (rl *IPRateLimiter) Close() error {
	if rl == nil || !rl.ownsBack || rl.backend == nil {
		return nil
	}
	return rl.backend.Close()
}

// UserRateLimiter applies cache-backed per-user window limits.
type UserRateLimiter struct {
	limiter  *WindowLimiter
	config   WindowConfig
	backend  cache.RawCache
	ownsBack bool
}

// NewUserRateLimiter creates a new cache-backed user rate limiter.
// If raw is nil, an in-memory frame cache is created.
func NewUserRateLimiter(raw cache.RawCache, config *WindowConfig) *UserRateLimiter {
	backend := raw
	owns := false
	if backend == nil {
		backend = cache.NewInMemoryCache()
		owns = true
	}

	cfg := normalizeWindowConfig(config)
	if cfg.KeyPrefix == defaultWindowPrefix {
		cfg.KeyPrefix = defaultUserPrefix
	}

	return &UserRateLimiter{
		limiter:  NewWindowLimiter(backend, &cfg),
		config:   cfg,
		backend:  backend,
		ownsBack: owns,
	}
}

// Allow checks whether a request from the given user should be allowed.
func (rl *UserRateLimiter) Allow(ctx context.Context, userID string) bool {
	if rl == nil || rl.limiter == nil {
		return true
	}
	return rl.limiter.Allow(ctx, userID)
}

// Close releases owned resources.
func (rl *UserRateLimiter) Close() error {
	if rl == nil || !rl.ownsBack || rl.backend == nil {
		return nil
	}
	return rl.backend.Close()
}

// GetIP extracts caller IP from request headers/remote address.
func GetIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}

	ip := util.GetIP(r)
	if ip == "" {
		return "unknown"
	}
	return ip
}

// GetUserID extracts user identity from frame auth claims in context.
func GetUserID(ctx context.Context) string {
	claims := security.ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}

	if claims.Subject != "" {
		return claims.Subject
	}

	return claims.GetAccessID()
}

// RateLimitMiddleware applies cache-backed IP rate limiting.
func RateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := GetIP(r)
			if !limiter.Allow(r.Context(), ip) {
				rateLimitedResponse(w, limiter.config, "ip")
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.config.MaxPerWindow))
			next.ServeHTTP(w, r)
		})
	}
}

// UserRateLimitMiddleware applies user-based limiting and falls back to IP for unauthenticated requests.
func UserRateLimitMiddleware(userLimiter *UserRateLimiter, ipLimiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userLimiter == nil && ipLimiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			userID := GetUserID(r.Context())
			if userID != "" && userLimiter != nil {
				w.Header().Set("X-RateLimit-Scope", "user")
				if !userLimiter.Allow(r.Context(), userID) {
					rateLimitedResponse(w, userLimiter.config, "user")
					return
				}
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", userLimiter.config.MaxPerWindow))
				next.ServeHTTP(w, r)
				return
			}

			if ipLimiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Scope", "ip")
			ip := GetIP(r)
			if !ipLimiter.Allow(r.Context(), ip) {
				rateLimitedResponse(w, ipLimiter.config, "ip")
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", ipLimiter.config.MaxPerWindow))
			next.ServeHTTP(w, r)
		})
	}
}

func rateLimitedResponse(w http.ResponseWriter, cfg WindowConfig, scope string) {
	retryAfter := int(math.Ceil(cfg.WindowDuration.Seconds()))
	if retryAfter <= 0 {
		retryAfter = int(time.Minute.Seconds())
	}

	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.MaxPerWindow))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Scope", scope)
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
	w.WriteHeader(http.StatusTooManyRequests)
	writeIgnoreErr(w, `{"error": "rate limit exceeded", "code": "rate_limit_exceeded"}`)
}

func writeIgnoreErr(w io.Writer, data string) {
	_, _ = fmt.Fprint(w, data)
}
