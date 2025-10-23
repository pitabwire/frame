package cache

import (
	"fmt"
	"sync"
)

// manager manages multiple raw cache instances.
type manager struct {
	caches sync.Map // map[string]RawCache
}

// NewManager creates a new manager.
func NewManager() Manager {
	return &manager{}
}

// AddCache adds a raw cache with the given name.
func (cm *manager) AddCache(name string, cache RawCache) {
	cm.caches.Store(name, cache)
}

// GetRawCache returns the raw cache with the given name.
func (cm *manager) GetRawCache(name string) (RawCache, bool) {
	c, ok := cm.caches.Load(name)
	if !ok {
		return nil, false
	}
	rawCache, ok := c.(RawCache)
	return rawCache, ok
}

// GetCache returns a typed cache with automatic serialization.
func GetCache[K comparable, V any](
	manager Manager,
	name string,
	keyFunc func(K) string,
) (Cache[K, V], bool) {
	raw, ok := manager.GetRawCache(name)
	if !ok {
		return nil, false
	}
	return NewGenericCache[K, V](raw, keyFunc), true
}

// RemoveCache removes and closes the cache with the given name.
func (cm *manager) RemoveCache(name string) error {
	c, ok := cm.caches.LoadAndDelete(name)
	if !ok {
		return nil
	}
	rawCache, ok := c.(RawCache)
	if !ok {
		return nil
	}
	return rawCache.Close()
}

// Close closes all managed caches.
func (cm *manager) Close() error {
	var errs []error

	cm.caches.Range(func(_, value interface{}) bool {
		if rawCache, ok := value.(RawCache); ok {
			if closeErr := rawCache.Close(); closeErr != nil {
				errs = append(errs, closeErr)
			}
		}
		return true
	})

	if len(errs) > 0 {
		return fmt.Errorf("errors closing caches: %v", errs)
	}
	return nil
}
