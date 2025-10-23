package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/frame/internal"
)

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
	raw     RawCache
	keyFunc func(K) string
}

// NewGenericCache creates a new generic cache with automatic serialization.
func NewGenericCache[K comparable, V any](raw RawCache, keyFunc func(K) string) Cache[K, V] {
	if keyFunc == nil {
		keyFunc = func(k K) string {
			return fmt.Sprintf("%v", k)
		}
	}
	return &GenericCache[K, V]{
		raw:     raw,
		keyFunc: keyFunc,
	}
}

func (g *GenericCache[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	var zero V
	data, found, err := g.raw.Get(ctx, g.keyFunc(key))
	if err != nil || !found {
		return zero, found, err
	}

	var value V
	if unmarshalErr := internal.Unmarshal(data, &value); unmarshalErr != nil {
		return zero, false, unmarshalErr
	}
	return value, true, nil
}

func (g *GenericCache[K, V]) Set(ctx context.Context, key K, value V, ttl time.Duration) error {
	data, err := internal.Marshal(value)
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
