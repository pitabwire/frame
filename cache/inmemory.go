package cache

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"sync/atomic"
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
	evictMu    sync.Mutex
	stopClean  chan struct{}
	cleanupInt time.Duration
	maxEntries int
	entryCount atomic.Int64
}

const defaultCleanupInterval = 5 * time.Minute
const defaultInMemoryCacheMaxEntries = 100000

// NewInMemoryCache creates a new in-memory cache.
func NewInMemoryCache() RawCache {
	cache := &InMemoryCache{
		stopClean:  make(chan struct{}),
		cleanupInt: defaultCleanupInterval,
		maxEntries: defaultInMemoryCacheMaxEntries,
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
			c.deleteKey(key)
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
		c.deleteKey(key)
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

	c.storeItem(key, item)
	return nil
}

// Expire updates the TTL of an existing key.
func (c *InMemoryCache) Expire(_ context.Context, key string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}

	for {
		value, ok := c.items.Load(key)
		if !ok {
			return nil
		}

		item, typeOK := value.(*inMemoryCacheItem)
		if !typeOK {
			return nil
		}

		if item.isExpired() {
			c.deleteKey(key)
			return nil
		}

		newItem := &inMemoryCacheItem{
			value:      item.value,
			expiration: time.Now().Add(ttl),
		}

		if c.items.CompareAndSwap(key, value, newItem) {
			return nil
		}
	}
}

func (c *InMemoryCache) SupportsPerKeyTTL() bool {
	return true
}

// Delete removes an item from the cache.
func (c *InMemoryCache) Delete(_ context.Context, key string) error {
	c.deleteKey(key)
	return nil
}

// Exists checks if a key exists in the cache.
func (c *InMemoryCache) Exists(_ context.Context, key string) (bool, error) {
	value, ok := c.items.Load(key)
	if !ok {
		return false, nil
	}

	if cachedItem, itemOK := value.(*inMemoryCacheItem); itemOK && cachedItem.isExpired() {
		c.deleteKey(key)
		return false, nil
	}

	return true, nil
}

// Flush clears all items from the cache.
func (c *InMemoryCache) Flush(_ context.Context) error {
	c.items = sync.Map{}
	c.entryCount.Store(0)
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

// readItemCounter reads and returns the counter value from a cache item,
// deleting expired items and returning 0 for them.
func (c *InMemoryCache) readItemCounter(key string, item *inMemoryCacheItem) int64 {
	if item.isExpired() {
		c.deleteKey(key)
		return 0
	}
	if len(item.value) < int64Size {
		return 0
	}
	uintVal := binary.BigEndian.Uint64(item.value)
	if uintVal > math.MaxInt64 {
		uintVal = math.MaxInt64
	}
	return int64(uintVal)
}

// Increment atomically increments a counter.
func (c *InMemoryCache) Increment(_ context.Context, key string, delta int64) (int64, error) {
	for {
		value, ok := c.items.Load(key)
		var currentVal int64
		var item *inMemoryCacheItem

		if ok {
			item, _ = value.(*inMemoryCacheItem)
		}

		if item != nil {
			currentVal = c.readItemCounter(key, item)
		}

		newVal := currentVal + delta
		newBytes := make([]byte, int64Size)
		if newVal < 0 {
			newVal = 0
		}
		binary.BigEndian.PutUint64(newBytes, uint64(newVal))

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
				c.entryCount.Add(1)
				c.evictIfNeeded()
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

func (c *InMemoryCache) storeItem(key string, item *inMemoryCacheItem) {
	if _, loaded := c.items.Load(key); !loaded {
		c.entryCount.Add(1)
	}

	c.items.Store(key, item)
	c.evictIfNeeded()
}

func (c *InMemoryCache) deleteKey(key interface{}) {
	if _, loaded := c.items.LoadAndDelete(key); loaded {
		c.entryCount.Add(-1)
	}
}

func (c *InMemoryCache) evictIfNeeded() {
	if c.maxEntries <= 0 || c.entryCount.Load() <= int64(c.maxEntries) {
		return
	}

	c.evictMu.Lock()
	defer c.evictMu.Unlock()

	for c.entryCount.Load() > int64(c.maxEntries) {
		var victim any
		c.items.Range(func(key, value interface{}) bool {
			item, ok := value.(*inMemoryCacheItem)
			if ok && item.isExpired() {
				victim = key
				return false
			}

			if victim == nil {
				victim = key
			}

			return victim == nil
		})

		if victim == nil {
			return
		}

		c.deleteKey(victim)
	}
}
