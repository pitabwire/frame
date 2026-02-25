package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type InMemoryInternalSuite struct {
	suite.Suite
}

func TestInMemoryInternalSuite(t *testing.T) {
	suite.Run(t, new(InMemoryInternalSuite))
}

func (s *InMemoryInternalSuite) TestExpireSupportsAndCleanup() {
	ctx := context.Background()
	raw := NewInMemoryCache()
	s.T().Cleanup(func() { _ = raw.Close() })

	mem, ok := raw.(*InMemoryCache)
	s.Require().True(ok)

	s.True(mem.SupportsPerKeyTTL())

	s.NoError(mem.Set(ctx, "expire_key", []byte("value"), 0))
	s.NoError(mem.Expire(ctx, "expire_key", 50*time.Millisecond))
	time.Sleep(80 * time.Millisecond)
	_, found, err := mem.Get(ctx, "expire_key")
	s.NoError(err)
	s.False(found)

	// Explicitly exercise cleanup path.
	mem.items.Store("stale", &inMemoryCacheItem{
		value:      []byte("x"),
		expiration: time.Now().Add(-time.Second),
	})
	mem.entryCount.Store(1)
	mem.cleanup()
	exists, err := mem.Exists(ctx, "stale")
	s.NoError(err)
	s.False(exists)
}
