package ratelimiter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pitabwire/frame/cache"
)

const (
	defaultWindowPrefix = "ratelimit"
	windowTLLOffset     = time.Second
)

var ErrCacheDoesNotSupportPerKeyTTL = errors.New("cache backend does not support per-key TTL")

// WindowConfig defines fixed-window counter limiter settings backed by cache.
type WindowConfig struct {
	WindowDuration time.Duration
	MaxPerWindow   int
	KeyPrefix      string
	FailOpen       bool
}

// DefaultWindowConfig returns conservative cache-backed limiter defaults.
func DefaultWindowConfig() *WindowConfig {
	return &WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   600,
		KeyPrefix:      defaultWindowPrefix,
		FailOpen:       true,
	}
}

// WindowLimiter enforces per-key fixed-window limits using atomic cache increments.
type WindowLimiter struct {
	cache  cache.RawCache
	config WindowConfig
}

// NewWindowLimiter creates a cache-backed window limiter.
func NewWindowLimiter(raw cache.RawCache, cfg *WindowConfig) (*WindowLimiter, error) {
	if raw == nil {
		return nil, errors.New("cache backend is required")
	}

	if !raw.SupportsPerKeyTTL() {
		return nil, ErrCacheDoesNotSupportPerKeyTTL
	}

	config := normalizeWindowConfig(cfg)
	return &WindowLimiter{cache: raw, config: config}, nil
}

// Allow checks whether key is still within configured window limit.
func (wl *WindowLimiter) Allow(ctx context.Context, key string) bool {
	if wl == nil || wl.cache == nil || wl.config.MaxPerWindow <= 0 {
		return true
	}

	bucketKey := wl.bucketKey(normalizeKey(key), time.Now().UTC())
	count, err := wl.cache.Increment(ctx, bucketKey, 1)
	if err != nil {
		return wl.config.FailOpen
	}

	if count == 1 {
		_ = wl.cache.Expire(ctx, bucketKey, wl.config.WindowDuration+windowTLLOffset)
	}

	return count <= int64(wl.config.MaxPerWindow)
}

func (wl *WindowLimiter) bucketKey(key string, now time.Time) string {
	windowSeconds := int64(wl.config.WindowDuration.Seconds())
	if windowSeconds <= 0 {
		windowSeconds = 60
	}
	bucket := now.Unix() / windowSeconds
	return fmt.Sprintf("%s:%s:%d", wl.config.KeyPrefix, key, bucket)
}

func normalizeWindowConfig(cfg *WindowConfig) WindowConfig {
	if cfg == nil {
		return *DefaultWindowConfig()
	}

	result := *cfg
	if result.WindowDuration <= 0 {
		result.WindowDuration = time.Minute
	}
	if result.MaxPerWindow <= 0 {
		result.MaxPerWindow = 600
	}
	if result.KeyPrefix == "" {
		result.KeyPrefix = defaultWindowPrefix
	}

	return result
}
