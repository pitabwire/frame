package ratelimiter

import (
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRequestsPerSecond = 100
	defaultBurstSize         = 200
	defaultCleanupInterval   = 5 * time.Minute
	defaultEntryTTL          = 10 * time.Minute
	defaultMaxEntries        = 100000
)

// RateLimiterConfig defines in-memory token bucket limiter settings.
type RateLimiterConfig struct {
	RequestsPerSecond int
	BurstSize         int
	CleanupInterval   time.Duration
	EntryTTL          time.Duration
	MaxEntries        int
}

// DefaultRateLimiterConfig returns conservative production defaults.
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		RequestsPerSecond: defaultRequestsPerSecond,
		BurstSize:         defaultBurstSize,
		CleanupInterval:   defaultCleanupInterval,
		EntryTTL:          defaultEntryTTL,
		MaxEntries:        defaultMaxEntries,
	}
}

type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess atomic.Int64
}

// KeyedLimiter applies token bucket limits independently per key (IP/user/tenant/etc).
type KeyedLimiter struct {
	mu      sync.RWMutex
	entries map[string]*limiterEntry
	config  RateLimiterConfig

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewKeyedLimiter creates a new keyed in-memory limiter and starts cleanup.
func NewKeyedLimiter(cfg *RateLimiterConfig) *KeyedLimiter {
	config := normalizeConfig(cfg)
	kl := &KeyedLimiter{
		entries: make(map[string]*limiterEntry),
		config:  config,
		stopCh:  make(chan struct{}),
	}

	go kl.cleanupLoop()
	return kl
}

// Allow checks and consumes a token for the supplied key.
func (k *KeyedLimiter) Allow(key string) bool {
	return k.AllowN(key, 1)
}

// AllowN checks and consumes n tokens for the supplied key.
func (k *KeyedLimiter) AllowN(key string, n int) bool {
	if n <= 0 {
		return true
	}

	entry := k.getOrCreateEntry(normalizeKey(key))
	entry.lastAccess.Store(time.Now().UnixNano())
	return entry.limiter.AllowN(time.Now(), n)
}

// Len returns the number of active keys.
func (k *KeyedLimiter) Len() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return len(k.entries)
}

// Close stops the cleanup goroutine.
func (k *KeyedLimiter) Close() error {
	k.stopOnce.Do(func() {
		close(k.stopCh)
	})
	return nil
}

func normalizeConfig(cfg *RateLimiterConfig) RateLimiterConfig {
	if cfg == nil {
		return *DefaultRateLimiterConfig()
	}

	result := *cfg
	if result.RequestsPerSecond <= 0 {
		result.RequestsPerSecond = defaultRequestsPerSecond
	}
	if result.BurstSize <= 0 {
		result.BurstSize = defaultBurstSize
	}
	if result.CleanupInterval <= 0 {
		result.CleanupInterval = defaultCleanupInterval
	}
	if result.EntryTTL <= 0 {
		result.EntryTTL = defaultEntryTTL
	}
	if result.MaxEntries <= 0 {
		result.MaxEntries = defaultMaxEntries
	}

	return result
}

func normalizeKey(key string) string {
	if key == "" {
		return "unknown"
	}
	return key
}

func (k *KeyedLimiter) getOrCreateEntry(key string) *limiterEntry {
	k.mu.RLock()
	entry, found := k.entries[key]
	k.mu.RUnlock()
	if found {
		return entry
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	entry, found = k.entries[key]
	if found {
		return entry
	}

	entry = &limiterEntry{
		limiter: rate.NewLimiter(rate.Limit(float64(k.config.RequestsPerSecond)), k.config.BurstSize),
	}
	entry.lastAccess.Store(time.Now().UnixNano())
	k.entries[key] = entry

	k.evictIfNeededLocked()
	return entry
}

func (k *KeyedLimiter) cleanupLoop() {
	ticker := time.NewTicker(k.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			k.cleanupExpired()
		case <-k.stopCh:
			return
		}
	}
}

func (k *KeyedLimiter) cleanupExpired() {
	cutoff := time.Now().Add(-k.config.EntryTTL).UnixNano()

	k.mu.Lock()
	defer k.mu.Unlock()

	for key, entry := range k.entries {
		if entry.lastAccess.Load() < cutoff {
			delete(k.entries, key)
		}
	}
}

func (k *KeyedLimiter) evictIfNeededLocked() {
	if len(k.entries) <= k.config.MaxEntries {
		return
	}

	for len(k.entries) > k.config.MaxEntries {
		oldestKey := ""
		oldest := int64(time.Now().UnixNano())
		for key, entry := range k.entries {
			last := entry.lastAccess.Load()
			if last <= oldest {
				oldest = last
				oldestKey = key
			}
		}

		if oldestKey == "" {
			return
		}
		delete(k.entries, oldestKey)
	}
}
