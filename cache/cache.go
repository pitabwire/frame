package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Serializer defines how values are serialized and deserialized.
type Serializer interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSONSerializer uses JSON encoding.
type JSONSerializer struct{}

func (s *JSONSerializer) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s *JSONSerializer) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// GobSerializer uses gob encoding.
type GobSerializer struct{}

func (s *GobSerializer) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *GobSerializer) Unmarshal(data []byte, v any) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(v)
}

// Cache is a generic cache interface with automatic serialization.
type Cache[K comparable, V any] interface {
	// Get retrieves an item from the cache
	Get(ctx context.Context, key K) (V, bool, error)

	// Set sets an item in the cache with the specified TTL
	Set(ctx context.Context, key K, value V, ttl time.Duration) error

	// Delete removes an item from the cache
	Delete(ctx context.Context, key K) error

	// Exists checks if a key exists in the cache
	Exists(ctx context.Context, key K) (bool, error)

	// Flush clears all items from the cache
	Flush(ctx context.Context) error

	// Close releases any resources used by the cache
	Close() error
}

// RawCache is the low-level cache interface that works with bytes.
type RawCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Flush(ctx context.Context) error
	Close() error
	Increment(ctx context.Context, key string, delta int64) (int64, error)
	Decrement(ctx context.Context, key string, delta int64) (int64, error)
}

// GenericCache wraps a RawCache and provides automatic serialization.
type GenericCache[K comparable, V any] struct {
	raw        RawCache
	serializer Serializer
	keyFunc    func(K) string
}

// NewGenericCache creates a new generic cache with automatic serialization.
func NewGenericCache[K comparable, V any](raw RawCache, serializer Serializer, keyFunc func(K) string) Cache[K, V] {
	if serializer == nil {
		serializer = &JSONSerializer{}
	}
	if keyFunc == nil {
		keyFunc = func(k K) string {
			return fmt.Sprintf("%v", k)
		}
	}
	return &GenericCache[K, V]{
		raw:        raw,
		serializer: serializer,
		keyFunc:    keyFunc,
	}
}

func (g *GenericCache[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	var zero V
	data, found, err := g.raw.Get(ctx, g.keyFunc(key))
	if err != nil || !found {
		return zero, found, err
	}

	var value V
	if unmarshalErr := g.serializer.Unmarshal(data, &value); unmarshalErr != nil {
		return zero, false, unmarshalErr
	}
	return value, true, nil
}

func (g *GenericCache[K, V]) Set(ctx context.Context, key K, value V, ttl time.Duration) error {
	data, err := g.serializer.Marshal(value)
	if err != nil {
		return err
	}
	return g.raw.Set(ctx, g.keyFunc(key), data, ttl)
}

func (g *GenericCache[K, V]) Delete(ctx context.Context, key K) error {
	return g.raw.Delete(ctx, g.keyFunc(key))
}

func (g *GenericCache[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	return g.raw.Exists(ctx, g.keyFunc(key))
}

func (g *GenericCache[K, V]) Flush(ctx context.Context) error {
	return g.raw.Flush(ctx)
}

func (g *GenericCache[K, V]) Close() error {
	return g.raw.Close()
}

// Manager manages multiple raw cache instances.
type Manager struct {
	caches sync.Map // map[string]RawCache
}

// NewManager creates a new Manager.
func NewManager() *Manager {
	return &Manager{}
}

// AddCache adds a raw cache with the given name.
func (cm *Manager) AddCache(name string, cache RawCache) {
	cm.caches.Store(name, cache)
}

// GetRawCache returns the raw cache with the given name.
func (cm *Manager) GetRawCache(name string) (RawCache, bool) {
	c, ok := cm.caches.Load(name)
	if !ok {
		return nil, false
	}
	rawCache, ok := c.(RawCache)
	return rawCache, ok
}

// GetCache returns a typed cache with automatic serialization.
func GetCache[K comparable, V any](
	manager *Manager,
	name string,
	serializer Serializer,
	keyFunc func(K) string,
) (Cache[K, V], bool) {
	raw, ok := manager.GetRawCache(name)
	if !ok {
		return nil, false
	}
	return NewGenericCache[K, V](raw, serializer, keyFunc), true
}

// RemoveCache removes and closes the cache with the given name.
func (cm *Manager) RemoveCache(name string) error {
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
func (cm *Manager) Close() error {
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
