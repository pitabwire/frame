package ratelimiter

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/security"
)

// IPRateLimiter applies keyed limits by client IP.
type IPRateLimiter struct {
	limiter *KeyedLimiter
	config  RateLimiterConfig
}

// NewIPRateLimiter creates a new IP-based rate limiter.
func NewIPRateLimiter(config *RateLimiterConfig) *IPRateLimiter {
	cfg := normalizeConfig(config)
	return &IPRateLimiter{
		limiter: NewKeyedLimiter(&cfg),
		config:  cfg,
	}
}

// Allow checks whether a request from the given IP should be allowed.
func (rl *IPRateLimiter) Allow(ip string) bool {
	if rl == nil || rl.limiter == nil {
		return true
	}
	return rl.limiter.Allow(ip)
}

// Close stops internal background cleanup.
func (rl *IPRateLimiter) Close() error {
	if rl == nil || rl.limiter == nil {
		return nil
	}
	return rl.limiter.Close()
}

// UserRateLimiter applies keyed limits by authenticated user identity.
type UserRateLimiter struct {
	limiter *KeyedLimiter
	config  RateLimiterConfig
}

// NewUserRateLimiter creates a new user-based rate limiter.
func NewUserRateLimiter(config *RateLimiterConfig) *UserRateLimiter {
	cfg := normalizeConfig(config)
	return &UserRateLimiter{
		limiter: NewKeyedLimiter(&cfg),
		config:  cfg,
	}
}

// Allow checks whether a request from the given user should be allowed.
func (rl *UserRateLimiter) Allow(userID string) bool {
	if rl == nil || rl.limiter == nil {
		return true
	}
	return rl.limiter.Allow(userID)
}

// Close stops internal background cleanup.
func (rl *UserRateLimiter) Close() error {
	if rl == nil || rl.limiter == nil {
		return nil
	}
	return rl.limiter.Close()
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

// RateLimitMiddleware applies IP-based rate limiting.
func RateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := GetIP(r)
			if !limiter.Allow(ip) {
				rateLimitedResponse(w, limiter.config.RequestsPerSecond, "ip")
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.config.RequestsPerSecond))
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
				if !userLimiter.Allow(userID) {
					rateLimitedResponse(w, userLimiter.config.RequestsPerSecond, "user")
					return
				}
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", userLimiter.config.RequestsPerSecond))
				next.ServeHTTP(w, r)
				return
			}

			if ipLimiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Scope", "ip")
			ip := GetIP(r)
			if !ipLimiter.Allow(ip) {
				rateLimitedResponse(w, ipLimiter.config.RequestsPerSecond, "ip")
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", ipLimiter.config.RequestsPerSecond))
			next.ServeHTTP(w, r)
		})
	}
}

func rateLimitedResponse(w http.ResponseWriter, limit int, scope string) {
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Scope", scope)
	w.Header().Set("Retry-After", "1")
	w.WriteHeader(http.StatusTooManyRequests)
	writeIgnoreErr(w, `{"error": "rate limit exceeded", "code": "rate_limit_exceeded"}`)
}

func writeIgnoreErr(w io.Writer, data string) {
	_, _ = fmt.Fprint(w, data)
}
