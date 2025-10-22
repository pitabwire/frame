package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pitabwire/frame/cache"
	cacheredis "github.com/pitabwire/frame/cache/redis"
	cachevalkey "github.com/pitabwire/frame/cache/valkey"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	testvalkey "github.com/pitabwire/frame/frametests/deps/testvalkey"
	"github.com/stretchr/testify/suite"
)

// CacheTestSuite runs all cache tests against different implementations.
type CacheTestSuite struct {
	frametests.FrameBaseTestSuite
	valkeyAddr string
}

func (s *CacheTestSuite) SetupSuite() {
	// Initialize resources for Valkey (used by both Redis and Valkey implementations)
	s.InitResourceFunc = func(_ context.Context) []definition.TestResource {
		return []definition.TestResource{
			testvalkey.New(),
		}
	}

	s.FrameBaseTestSuite.SetupSuite()

	// Get Valkey connection string from resources
	resources := s.Resources()
	for _, res := range resources {
		ds := res.GetDS(s.T().Context())
		if ds.IsCache() {
			s.valkeyAddr = ds.String()
			s.T().Logf("Valkey connection string: %s", s.valkeyAddr)
			break
		}
	}
}

func TestCacheTestSuite(t *testing.T) {
	suite.Run(t, new(CacheTestSuite))
}

// getCacheImplementations returns all cache implementations to test.
func (s *CacheTestSuite) getCacheImplementations() map[string]cache.RawCache {
	implementations := make(map[string]cache.RawCache)

	// Always add in-memory
	implementations["InMemory"] = cache.NewInMemoryCache()

	// Add Valkey if available
	if s.valkeyAddr != "" {
		valkeyCache, err := cachevalkey.New(cachevalkey.Options{
			Addr: s.valkeyAddr,
			DB:   0,
		})
		if err == nil {
			implementations["Valkey"] = valkeyCache
		} else {
			s.T().Logf("Valkey not available: %v", err)
		}

		// Add Redis (using same Valkey server as it's compatible)
		// Redis client needs the address without the redis:// scheme
		redisCache, err := cacheredis.New(cacheredis.Options{
			Addr: s.valkeyAddr, // cacheredis.New handles URL parsing
			DB:   1,
		})
		if err == nil {
			implementations["Redis"] = redisCache
		} else {
			s.T().Logf("Redis not available: %v", err)
		}
	}

	return implementations
}

// Test Basic Operations.
func (s *CacheTestSuite) TestBasicOperations() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			tests := []struct {
				testName string
				key      string
				value    []byte
				ttl      time.Duration
			}{
				{"Simple value", "key1", []byte("value1"), 0},
				{"With TTL", "key2", []byte("value2"), 1 * time.Hour},
				{"Empty value", "key3", []byte{}, 0},
				{"Large value", "key4", make([]byte, 1024), 0},
			}

			for _, tt := range tests {
				s.Run(tt.testName, func() {
					// Set
					err := rawCache.Set(ctx, tt.key, tt.value, tt.ttl)
					s.Require().NoError(err)

					// Get
					value, found, err := rawCache.Get(ctx, tt.key)
					s.Require().NoError(err)
					s.True(found)
					s.Equal(tt.value, value)

					// Exists
					exists, err := rawCache.Exists(ctx, tt.key)
					s.Require().NoError(err)
					s.True(exists)

					// Delete
					err = rawCache.Delete(ctx, tt.key)
					s.Require().NoError(err)

					// Verify deletion
					_, found, err = rawCache.Get(ctx, tt.key)
					s.Require().NoError(err)
					s.False(found)
				})
			}
		})
	}
}

// Test TTL Expiration.
func (s *CacheTestSuite) TestTTLExpiration() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			key := "expiring-key"
			value := []byte("expiring-value")

			// Set with short TTL (use 1 second for compatibility with all backends)
			ttl := 1 * time.Second
			err := rawCache.Set(ctx, key, value, ttl)
			s.Require().NoError(err)

			// Should exist immediately
			_, found, err := rawCache.Get(ctx, key)
			s.Require().NoError(err)
			s.True(found)

			// Wait for expiration
			time.Sleep(ttl + 200*time.Millisecond)

			// Should be expired
			_, found, err = rawCache.Get(ctx, key)
			s.Require().NoError(err)
			s.False(found)
		})
	}
}

// Test Counter Operations.
func (s *CacheTestSuite) TestCounterOperations() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			key := "counter"

			// Increment from zero
			val, err := rawCache.Increment(ctx, key, 1)
			s.Require().NoError(err)
			s.Equal(int64(1), val)

			// Increment by 5
			val, err = rawCache.Increment(ctx, key, 5)
			s.Require().NoError(err)
			s.Equal(int64(6), val)

			// Decrement by 2
			val, err = rawCache.Decrement(ctx, key, 2)
			s.Require().NoError(err)
			s.Equal(int64(4), val)

			// Increment by negative (same as decrement)
			val, err = rawCache.Increment(ctx, key, -1)
			s.Require().NoError(err)
			s.Equal(int64(3), val)

			// Clean up
			_ = rawCache.Delete(ctx, key)
		})
	}
}

// Test Concurrent Access.
//
//nolint:gocognit // Complex due to concurrent goroutine coordination
func (s *CacheTestSuite) TestConcurrentAccess() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			const goroutines = 50
			const iterations = 10

			var wg sync.WaitGroup
			wg.Add(goroutines)

			// Concurrent writes
			for i := range goroutines {
				go func(id int) {
					defer wg.Done()
					for j := range iterations {
						key := fmt.Sprintf("concurrent:%s:key:%d:%d", name, id, j)
						value := []byte(fmt.Sprintf("value-%d-%d", id, j))
						_ = rawCache.Set(ctx, key, value, 1*time.Hour)
					}
				}(i)
			}

			wg.Wait()

			// Verify writes
			for i := range goroutines {
				for j := range iterations {
					key := fmt.Sprintf("concurrent:%s:key:%d:%d", name, i, j)
					_, found, err := rawCache.Get(ctx, key)
					s.Require().NoError(err)
					s.True(found, "Key %s should exist", key)
				}
			}

			// Clean up
			for i := range goroutines {
				for j := range iterations {
					key := fmt.Sprintf("concurrent:%s:key:%d:%d", name, i, j)
					_ = rawCache.Delete(ctx, key)
				}
			}
		})
	}
}

// Test Concurrent Counters.
func (s *CacheTestSuite) TestConcurrentCounters() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			const goroutines = 50
			const incrementsPerGoroutine = 100
			key := fmt.Sprintf("concurrent-counter:%s", name)

			var wg sync.WaitGroup
			wg.Add(goroutines)

			// Concurrent increments
			for range goroutines {
				go func() {
					defer wg.Done()
					for range incrementsPerGoroutine {
						_, _ = rawCache.Increment(ctx, key, 1)
					}
				}()
			}

			wg.Wait()

			// Verify final count
			expectedCount := int64(goroutines * incrementsPerGoroutine)
			finalVal, err := rawCache.Increment(ctx, key, 0)
			s.Require().NoError(err)
			s.Equal(expectedCount, finalVal)

			// Clean up
			_ = rawCache.Delete(ctx, key)
		})
	}
}

// Test Flush Operation.
func (s *CacheTestSuite) TestFlush() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			// Add items with unique prefix for this test
			keyPrefix := fmt.Sprintf("flush:%s:key:", name)
			for i := range 5 {
				key := fmt.Sprintf("%s%d", keyPrefix, i)
				_ = rawCache.Set(ctx, key, []byte(fmt.Sprintf("value-%d", i)), 0)
			}

			// Verify exists
			exists, _ := rawCache.Exists(ctx, keyPrefix+"0")
			s.True(exists)

			// Flush
			err := rawCache.Flush(ctx)
			s.Require().NoError(err)

			// Verify gone
			exists, _ = rawCache.Exists(ctx, keyPrefix+"0")
			s.False(exists)
		})
	}
}

// Test Edge Cases.
func (s *CacheTestSuite) TestEdgeCases() {
	ctx := context.Background()

	for name, rawCache := range s.getCacheImplementations() {
		s.Run(name, func() {
			defer rawCache.Close()

			keyPrefix := fmt.Sprintf("edge:%s:", name)

			// Get non-existent key
			_, found, err := rawCache.Get(ctx, keyPrefix+"nonexistent")
			s.Require().NoError(err)
			s.False(found)

			// Delete non-existent key
			err = rawCache.Delete(ctx, keyPrefix+"nonexistent")
			s.Require().NoError(err)

			// Exists for non-existent key
			exists, err := rawCache.Exists(ctx, keyPrefix+"nonexistent")
			s.Require().NoError(err)
			s.False(exists)

			// Nil/empty value
			err = rawCache.Set(ctx, keyPrefix+"nil-key", nil, 0)
			s.Require().NoError(err)

			value, found, err := rawCache.Get(ctx, keyPrefix+"nil-key")
			s.Require().NoError(err)
			s.True(found)
			// Accept both nil and empty byte slice
			s.Empty(value, "Value should be empty")

			// Clean up
			_ = rawCache.Delete(ctx, keyPrefix+"nil-key")
		})
	}
}

// Test Generic Cache with Serialization.
func (s *CacheTestSuite) TestGenericCache() {
	ctx := context.Background()

	type User struct {
		ID   string
		Name string
	}

	manager := cache.NewManager()
	defer manager.Close()

	// Test with in-memory cache
	manager.AddCache("users", cache.NewInMemoryCache())

	// Get typed cache
	userCache, ok := cache.GetCache[string, User](manager, "users", nil, nil)
	s.True(ok)

	// Store user
	user := User{ID: "123", Name: "John"}
	err := userCache.Set(ctx, user.ID, user, 1*time.Hour)
	s.Require().NoError(err)

	// Retrieve user
	cachedUser, found, err := userCache.Get(ctx, "123")
	s.Require().NoError(err)
	s.True(found)
	s.Equal(user.Name, cachedUser.Name)

	// Delete
	err = userCache.Delete(ctx, "123")
	s.Require().NoError(err)

	_, found, _ = userCache.Get(ctx, "123")
	s.False(found)
}

// Test Cache Manager.
func (s *CacheTestSuite) TestCacheManager() {
	ctx := context.Background()

	manager := cache.NewManager()
	defer manager.Close()

	// Add multiple caches
	manager.AddCache("cache1", cache.NewInMemoryCache())
	manager.AddCache("cache2", cache.NewInMemoryCache())

	cache1, ok1 := manager.GetRawCache("cache1")
	cache2, ok2 := manager.GetRawCache("cache2")
	s.True(ok1)
	s.True(ok2)

	// Set different values
	_ = cache1.Set(ctx, "key", []byte("value1"), 0)
	_ = cache2.Set(ctx, "key", []byte("value2"), 0)

	// Verify isolation
	val1, _, _ := cache1.Get(ctx, "key")
	val2, _, _ := cache2.Get(ctx, "key")
	s.Equal([]byte("value1"), val1)
	s.Equal([]byte("value2"), val2)

	// Remove cache
	err := manager.RemoveCache("cache1")
	s.Require().NoError(err)

	_, ok := manager.GetRawCache("cache1")
	s.False(ok)
}
