package cache

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/pitabwire/frame/v2/internal"
	"github.com/pitabwire/frame/v2/tenancy"
)

const maxKeySegmentLen = 64

// TenantCacheOption configures a tenant-aware cache wrapper.
type TenantCacheOption func(*tenantCacheConfig)

type tenantCacheConfig struct {
	globalNamespace bool
	// secureMode when true refuses writes/reads without a tenant (no unset/ fallback).
	secureMode bool
}

// WithGlobalNamespace forces the g/ prefix for shared lookups (JWKS, flags).
func WithGlobalNamespace() TenantCacheOption {
	return func(c *tenantCacheConfig) {
		c.globalNamespace = true
	}
}

// WithSecureCacheMode refuses cache ops without a bindable tenant
// (Secure Profile). Default false uses unset/ when claims are missing.
func WithSecureCacheMode(secure bool) TenantCacheOption {
	return func(c *tenantCacheConfig) {
		c.secureMode = secure
	}
}

// TenantKeyPrefix returns the cache key namespace for ctx per Secure Profile
// rules (K25/K31):
//
//	g/     — explicit WithGlobalNamespace on the cache instance (caller sets)
//	t/id/  — bindable TenantID from Claims or scoped SystemPrincipal
//	sys/s/ — SystemPrincipal without TenantID
//	unset/ — FailOpen missing claims
//
// globalNamespace=true forces g/.
func TenantKeyPrefix(ctx context.Context, globalNamespace, secureMode bool) (string, error) {
	if globalNamespace {
		return "g/", nil
	}

	// Prefer bindable tenant from claims or scoped principal.
	if tid := bindableTenantID(ctx); tid != "" {
		seg, err := sanitizeKeySegment(tid)
		if err != nil {
			return "", err
		}
		return "t/" + seg + "/", nil
	}

	if sp, ok := tenancy.SystemPrincipalFromContext(ctx); ok {
		name := sp.ServiceName
		if name == "" {
			name = "system"
		}
		seg, err := sanitizeKeySegment(name)
		if err != nil {
			return "", err
		}
		return "sys/" + seg + "/", nil
	}

	if secureMode {
		return "", tenancy.ErrCacheTenantRequired
	}
	return "unset/", nil
}

func bindableTenantID(ctx context.Context) string {
	if sp, ok := tenancy.SystemPrincipalFromContext(ctx); ok {
		if tid := strings.TrimSpace(sp.TenantID); tid != "" && !sp.AllowGlobal {
			return tid
		}
		// AllowGlobal with empty TenantID → sys/; with TenantID still prefer t/
		if tid := strings.TrimSpace(sp.TenantID); tid != "" {
			return tid
		}
	}
	if c := tenancy.ClaimsFromContext(ctx); c != nil {
		if c.IsBindable() {
			return strings.TrimSpace(c.TenantID)
		}
	}
	return ""
}

func sanitizeKeySegment(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: empty segment", tenancy.ErrInvalidCacheKeySegment)
	}
	if len(s) > maxKeySegmentLen {
		return "", fmt.Errorf("%w: segment too long", tenancy.ErrInvalidCacheKeySegment)
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r == 0 || unicode.IsControl(r) {
			return "", fmt.Errorf("%w: illegal character in %q", tenancy.ErrInvalidCacheKeySegment, s)
		}
	}
	return s, nil
}

// TenantAwareRaw wraps RawCache with automatic tenant key prefixes.
type TenantAwareRaw struct {
	raw    RawCache
	global bool
	secure bool
}

// NewTenantAwareRaw wraps raw with tenant prefixes. Prefix is off only when
// this wrapper is not used — call sites opt in via NewTenantAwareRaw.
func NewTenantAwareRaw(raw RawCache, opts ...TenantCacheOption) *TenantAwareRaw {
	cfg := tenantCacheConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return &TenantAwareRaw{raw: raw, global: cfg.globalNamespace, secure: cfg.secureMode}
}

func (t *TenantAwareRaw) prefix(ctx context.Context, key string) (string, error) {
	p, err := TenantKeyPrefix(ctx, t.global, t.secure)
	if err != nil {
		return "", err
	}
	return p + key, nil
}

func (t *TenantAwareRaw) Get(ctx context.Context, key string) ([]byte, bool, error) {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return nil, false, err
	}
	return t.raw.Get(ctx, k)
}

func (t *TenantAwareRaw) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return err
	}
	return t.raw.Set(ctx, k, value, ttl)
}

func (t *TenantAwareRaw) Expire(ctx context.Context, key string, ttl time.Duration) error {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return err
	}
	return t.raw.Expire(ctx, k, ttl)
}

func (t *TenantAwareRaw) SupportsPerKeyTTL() bool { return t.raw.SupportsPerKeyTTL() }

func (t *TenantAwareRaw) Delete(ctx context.Context, key string) error {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return err
	}
	return t.raw.Delete(ctx, k)
}

func (t *TenantAwareRaw) Exists(ctx context.Context, key string) (bool, error) {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return false, err
	}
	return t.raw.Exists(ctx, k)
}

func (t *TenantAwareRaw) Flush(ctx context.Context) error { return t.raw.Flush(ctx) }

func (t *TenantAwareRaw) Close() error { return t.raw.Close() }

func (t *TenantAwareRaw) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return 0, err
	}
	return t.raw.Increment(ctx, k, delta)
}

func (t *TenantAwareRaw) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	k, err := t.prefix(ctx, key)
	if err != nil {
		return 0, err
	}
	return t.raw.Decrement(ctx, k, delta)
}

// NewTenantAwareCache builds a generic cache with tenant key prefixes.
func NewTenantAwareCache[K comparable, V any](
	raw RawCache,
	keyFunc func(K) string,
	opts ...TenantCacheOption,
) Cache[K, V] {
	wrapped := NewTenantAwareRaw(raw, opts...)
	if keyFunc == nil {
		keyFunc = func(k K) string { return fmt.Sprintf("%v", k) }
	}
	return &tenantGenericCache[K, V]{
		raw:     wrapped,
		keyFunc: keyFunc,
	}
}

type tenantGenericCache[K comparable, V any] struct {
	raw     *TenantAwareRaw
	keyFunc func(K) string
}

func (g *tenantGenericCache[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	var zero V
	data, found, err := g.raw.Get(ctx, g.keyFunc(key))
	if err != nil || !found {
		return zero, found, err
	}
	var value V
	if uerr := internal.Unmarshal(data, &value); uerr != nil {
		return zero, false, uerr
	}
	return value, true, nil
}

func (g *tenantGenericCache[K, V]) Set(ctx context.Context, key K, value V, ttl time.Duration) error {
	data, err := internal.Marshal(value)
	if err != nil {
		return err
	}
	return g.raw.Set(ctx, g.keyFunc(key), data, ttl)
}

func (g *tenantGenericCache[K, V]) Delete(ctx context.Context, key K) error {
	return g.raw.Delete(ctx, g.keyFunc(key))
}

func (g *tenantGenericCache[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	return g.raw.Exists(ctx, g.keyFunc(key))
}

func (g *tenantGenericCache[K, V]) Flush(ctx context.Context) error { return g.raw.Flush(ctx) }

func (g *tenantGenericCache[K, V]) Close() error { return g.raw.Close() }

var _ RawCache = (*TenantAwareRaw)(nil)
