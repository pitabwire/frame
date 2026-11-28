package cache_test

import (
	"testing"

	"github.com/pitabwire/frame/v3/cache"
	"github.com/pitabwire/frame/v3/cache/jetstreamkv"
	"github.com/pitabwire/frame/v3/cache/redis"
	"github.com/pitabwire/frame/v3/cache/valkey"
	"github.com/pitabwire/frame/v3/data"
	"github.com/stretchr/testify/require"
)

// Cache constructors must not surface the DSN password in their errors:
// URL parsers and drivers echo the full connection string.
func TestCacheNew_ErrorsNeverLeakDSNCredentials(t *testing.T) {
	t.Parallel()
	const password = "cache-pw-31337"

	cases := []struct {
		name string
		dsn  string
		open func(...cache.Option) (cache.RawCache, error)
	}{
		{name: "redis unparseable dsn", dsn: "redis://svc:" + password + "@host:6379/\x7f", open: redis.New},
		{name: "valkey unparseable dsn", dsn: "redis://svc:" + password + "@host:6379/\x7f", open: valkey.New},
		{name: "jetstream unparseable dsn", dsn: "nats://svc:" + password + "@host:4222/\x7f", open: jetstreamkv.New},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.open(cache.WithDSN(data.DSN(tc.dsn)), cache.WithName("bucket"))
			require.Error(t, err)
			require.NotContains(t, err.Error(), password)
		})
	}
}
