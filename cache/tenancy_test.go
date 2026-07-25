package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/pitabwire/frame/v2/cache"
	"github.com/pitabwire/frame/v2/tenancy"
	"github.com/stretchr/testify/require"
)

type memRaw struct {
	m map[string][]byte
}

func newMemRaw() *memRaw { return &memRaw{m: map[string][]byte{}} }

func (m *memRaw) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m.m[key]
	return v, ok, nil
}
func (m *memRaw) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.m[key] = value
	return nil
}
func (m *memRaw) Expire(context.Context, string, time.Duration) error { return nil }
func (m *memRaw) SupportsPerKeyTTL() bool                             { return true }
func (m *memRaw) Delete(_ context.Context, key string) error {
	delete(m.m, key)
	return nil
}
func (m *memRaw) Exists(_ context.Context, key string) (bool, error) {
	_, ok := m.m[key]
	return ok, nil
}
func (m *memRaw) Flush(context.Context) error { return nil }
func (m *memRaw) Close() error                { return nil }
func (m *memRaw) Increment(context.Context, string, int64) (int64, error) {
	return 0, nil
}
func (m *memRaw) Decrement(context.Context, string, int64) (int64, error) {
	return 0, nil
}

func TestTenantKeyPrefix(t *testing.T) {
	t.Parallel()

	p, err := cache.TenantKeyPrefix(context.Background(), true, false)
	require.NoError(t, err)
	require.Equal(t, "g/", p)

	ctx := tenancy.WithClaims(context.Background(), &tenancy.Claims{TenantID: "t1"})
	p, err = cache.TenantKeyPrefix(ctx, false, false)
	require.NoError(t, err)
	require.Equal(t, "t/t1/", p)

	ctx = tenancy.WithSystemPrincipal(context.Background(), tenancy.SystemPrincipal{
		ServiceName: "job", TenantID: "t2",
	})
	p, err = cache.TenantKeyPrefix(ctx, false, false)
	require.NoError(t, err)
	require.Equal(t, "t/t2/", p)

	ctx = tenancy.WithSystemPrincipal(context.Background(), tenancy.SystemPrincipal{
		ServiceName: "admin", AllowGlobal: true,
	})
	p, err = cache.TenantKeyPrefix(ctx, false, false)
	require.NoError(t, err)
	require.Equal(t, "sys/admin/", p)

	p, err = cache.TenantKeyPrefix(context.Background(), false, false)
	require.NoError(t, err)
	require.Equal(t, "unset/", p)

	_, err = cache.TenantKeyPrefix(context.Background(), false, true)
	require.ErrorIs(t, err, tenancy.ErrCacheTenantRequired)
}

func TestTenantAwareRawPrefixesKeys(t *testing.T) {
	t.Parallel()
	raw := newMemRaw()
	ta := cache.NewTenantAwareRaw(raw)
	ctx := tenancy.WithClaims(context.Background(), &tenancy.Claims{TenantID: "acme"})
	require.NoError(t, ta.Set(ctx, "k", []byte("v"), time.Minute))
	require.Contains(t, raw.m, "t/acme/k")
	got, ok, err := ta.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []byte("v"), got)
}
