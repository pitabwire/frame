package ratelimiter

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/pitabwire/frame/cache"
)

const (
	defaultWindowPrefix  = "ratelimit"
	windowTLLOffset      = time.Second
	defaultMaxPerWindow  = 600
	bucketKeyEstOverhead = 24
	decimalBase          = 10
	reserveCapDivisor    = 10
	largeWindowLimit     = 1_000_000
	mediumWindowLimit    = 100_000
	smallWindowLimit     = 10_000
	tinyWindowLimit      = 1_000
	microWindowLimit     = 100
	largeReservation     = 4096
	mediumReservation    = 1024
	smallReservation     = 256
	tinyReservation      = 64
	microReservation     = 16
)

var ErrCacheDoesNotSupportPerKeyTTL = errors.New("cache backend does not support per-key TTL")

// WindowConfig defines fixed-window counter limiter settings backed by cache.
type WindowConfig struct {
	WindowDuration  time.Duration
	MaxPerWindow    int
	KeyPrefix       string
	FailOpen        bool
	ReservationSize int
}

// DefaultWindowConfig returns conservative cache-backed limiter defaults.
func DefaultWindowConfig() *WindowConfig {
	return &WindowConfig{
		WindowDuration: time.Minute,
		MaxPerWindow:   defaultMaxPerWindow,
		KeyPrefix:      defaultWindowPrefix,
		FailOpen:       false,
	}
}

// WindowLimiter enforces per-key fixed-window limits using atomic cache increments.
type WindowLimiter struct {
	cache  cache.RawCache
	config WindowConfig
}

// LeasedWindowLimiter enforces fixed-window limits using chunk reservations
// from cache to reduce hot-key pressure at high request volume.
type LeasedWindowLimiter struct {
	cache  cache.RawCache
	config WindowConfig

	leases sync.Map
}

type leaseState struct {
	mu        sync.Mutex
	windowKey int64
	remaining int64
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

// NewLeasedWindowLimiter creates a cache-backed fixed-window limiter that
// reserves quota from the backend in chunks instead of one increment per call.
func NewLeasedWindowLimiter(raw cache.RawCache, cfg *WindowConfig) (*LeasedWindowLimiter, error) {
	if raw == nil {
		return nil, errors.New("cache backend is required")
	}

	if !raw.SupportsPerKeyTTL() {
		return nil, ErrCacheDoesNotSupportPerKeyTTL
	}

	config := normalizeWindowConfig(cfg)
	return &LeasedWindowLimiter{cache: raw, config: config}, nil
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

// Allow checks whether key is still within configured window limit.
func (ll *LeasedWindowLimiter) Allow(ctx context.Context, key string) bool {
	if ll == nil || ll.cache == nil || ll.config.MaxPerWindow <= 0 {
		return true
	}

	normalizedKey := normalizeKey(key)
	windowID := currentWindowID(ll.config.WindowDuration, time.Now().UTC())
	state := ll.stateForKey(normalizedKey)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.windowKey != windowID {
		state.windowKey = windowID
		state.remaining = 0
	}

	if state.remaining > 0 {
		state.remaining--
		return true
	}

	return ll.reserveLocked(ctx, state, normalizedKey, windowID)
}

func (wl *WindowLimiter) bucketKey(key string, now time.Time) string {
	bucket := currentWindowID(wl.config.WindowDuration, now)

	// Single allocation for final string.
	buf := make([]byte, 0, len(wl.config.KeyPrefix)+len(key)+bucketKeyEstOverhead)
	buf = append(buf, wl.config.KeyPrefix...)
	buf = append(buf, ':')
	buf = append(buf, key...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, bucket, decimalBase)
	return string(buf)
}

func (ll *LeasedWindowLimiter) bucketKey(key string, windowID int64) string {
	buf := make([]byte, 0, len(ll.config.KeyPrefix)+len(key)+bucketKeyEstOverhead)
	buf = append(buf, ll.config.KeyPrefix...)
	buf = append(buf, ':')
	buf = append(buf, key...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, windowID, decimalBase)
	return string(buf)
}

func (ll *LeasedWindowLimiter) stateForKey(key string) *leaseState {
	if existing, found := ll.leases.Load(key); found {
		if existingState, typed := existing.(*leaseState); typed {
			return existingState
		}
	}

	state := &leaseState{}
	actual, _ := ll.leases.LoadOrStore(key, state)
	if actualState, typed := actual.(*leaseState); typed {
		return actualState
	}

	return state
}

func (ll *LeasedWindowLimiter) reserveLocked(
	ctx context.Context,
	state *leaseState,
	key string,
	windowID int64,
) bool {
	reservation := ll.config.ReservationSize
	if reservation <= 0 {
		reservation = 1
	}

	count, err := ll.cache.Increment(ctx, ll.bucketKey(key, windowID), int64(reservation))
	if err != nil {
		return ll.config.FailOpen
	}

	if count == int64(reservation) {
		_ = ll.cache.Expire(ctx, ll.bucketKey(key, windowID), ll.config.WindowDuration+windowTLLOffset)
	}

	previousReserved := count - int64(reservation)
	if previousReserved >= int64(ll.config.MaxPerWindow) {
		return false
	}

	granted := int64(ll.config.MaxPerWindow) - previousReserved
	if granted > int64(reservation) {
		granted = int64(reservation)
	}

	if granted <= 0 {
		return false
	}

	state.remaining = granted - 1
	return true
}

func currentWindowID(window time.Duration, now time.Time) int64 {
	windowSeconds := int64(window.Seconds())
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	return now.Unix() / windowSeconds
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
		result.MaxPerWindow = defaultMaxPerWindow
	}
	if result.KeyPrefix == "" {
		result.KeyPrefix = defaultWindowPrefix
	}
	if result.ReservationSize <= 0 {
		result.ReservationSize = defaultReservationSize(result.MaxPerWindow)
	}
	if result.ReservationSize > result.MaxPerWindow {
		result.ReservationSize = result.MaxPerWindow
	}
	if maxReservation := result.MaxPerWindow / reserveCapDivisor; maxReservation > 0 &&
		result.ReservationSize > maxReservation {
		result.ReservationSize = maxReservation
	}
	if result.ReservationSize <= 0 {
		result.ReservationSize = 1
	}

	return result
}

func defaultReservationSize(maxPerWindow int) int {
	switch {
	case maxPerWindow >= largeWindowLimit:
		return largeReservation
	case maxPerWindow >= mediumWindowLimit:
		return mediumReservation
	case maxPerWindow >= smallWindowLimit:
		return smallReservation
	case maxPerWindow >= tinyWindowLimit:
		return tinyReservation
	case maxPerWindow >= microWindowLimit:
		return microReservation
	default:
		return 1
	}
}
