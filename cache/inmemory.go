package cache

import (
	"context"
	"encoding/binary"
	"sync"
	"time"
)

// inMemoryCacheItem represents a cache item with expiration.
type inMemoryCacheItem struct {
	value      []byte
	expiration time.Time
}

// isExpired checks if the item has expired.
func (i *inMemoryCacheItem) isExpired() bool {
	if i.expiration.IsZero() {
		return false
	}
	return time.Now().After(i.expiration)
}

// InMemoryCache is a thread-safe in-memory cache implementation.
type InMemoryCache struct {
	items      sync.Map // map[string]*inMemoryCacheItem
	cleanupMu  sync.Mutex
	stopClean  chan struct{}
	cleanupInt time.Duration
}

const defaultCleanupInterval = 5 * time.Minute

// NewInMemoryCache creates a new in-memory cache.
func NewInMemoryCache() RawCache {
	cache := &InMemoryCache{
		stopClean:  make(chan struct{}),
		cleanupInt: defaultCleanupInterval,
	}

	// Start cleanup goroutine
	go cache.startCleanup()

	return cache
}

// startCleanup periodically removes expired items.
func (c *InMemoryCache) startCleanup() {
	ticker := time.NewTicker(c.cleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopClean:
			return
		}
	}
}

// cleanup removes expired items from the cache.
func (c *InMemoryCache) cleanup() {
	c.items.Range(func(key, value interface{}) bool {
		item, ok := value.(*inMemoryCacheItem)
		if ok && item.isExpired() {
			c.items.Delete(key)
		}
		return true
	})
}

// Get retrieves an item from the cache.
func (c *InMemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := c.items.Load(key)
	if !ok {
		return nil, false, nil
	}

	item, ok := value.(*inMemoryCacheItem)
	if !ok || item.isExpired() {
		c.items.Delete(key)
		return nil, false, nil
	}

	return item.value, true, nil
}

// Set sets an item in the cache with the specified TTL.
func (c *InMemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	item := &inMemoryCacheItem{
		value: value,
	}

	if ttl > 0 {
		item.expiration = time.Now().Add(ttl)
	}

	c.items.Store(key, item)
	return nil
}

// Delete removes an item from the cache.
func (c *InMemoryCache) Delete(_ context.Context, key string) error {
	c.items.Delete(key)
	return nil
}

// Exists checks if a key exists in the cache.
func (c *InMemoryCache) Exists(_ context.Context, key string) (bool, error) {
	value, ok := c.items.Load(key)
	if !ok {
		return false, nil
	}

	if cachedItem, itemOK := value.(*inMemoryCacheItem); itemOK && cachedItem.isExpired() {
		c.items.Delete(key)
		return false, nil
	}

	return true, nil
}

// Flush clears all items from the cache.
func (c *InMemoryCache) Flush(_ context.Context) error {
	c.items = sync.Map{}
	return nil
}

// Close stops the cleanup goroutine and releases resources.
func (c *InMemoryCache) Close() error {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()

	select {
	case <-c.stopClean:
		// Already closed
		return nil
	default:
		close(c.stopClean)
	}

	return nil
}

const int64Size = 8

// Increment atomically increments a counter.
//
//nolint:gocognit // Complex due to atomic CAS retry loop
func (c *InMemoryCache) Increment(_ context.Context, key string, delta int64) (int64, error) {
	for {
		value, ok := c.items.Load(key)
		var currentVal int64
		var item *inMemoryCacheItem

		if ok {
			if cachedItem, typeOK := value.(*inMemoryCacheItem); typeOK {
				item = cachedItem
				if item.isExpired() {
					c.items.Delete(key)
					currentVal = 0
				} else if len(item.value) >= int64Size {
					uintVal := binary.BigEndian.Uint64(item.value)
					currentVal = int64(uintVal) //nolint:gosec // Safe conversion for counter values
				}
			}
		}

		newVal := currentVal + delta
		newBytes := make([]byte, int64Size)
		binary.BigEndian.PutUint64(newBytes, uint64(newVal)) //nolint:gosec // Safe conversion for counter values

		newItem := &inMemoryCacheItem{
			value: newBytes,
		}

		if item != nil && !item.expiration.IsZero() {
			newItem.expiration = item.expiration
		}

		// Use CompareAndSwap for atomic update
		if ok {
			if c.items.CompareAndSwap(key, value, newItem) {
				return newVal, nil
			}
		} else {
			if _, loaded := c.items.LoadOrStore(key, newItem); !loaded {
				return newVal, nil
			}
		}
		// Retry if CAS failed
	}
}

// Decrement atomically decrements a counter.
func (c *InMemoryCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return c.Increment(ctx, key, -delta)
}
