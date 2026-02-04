package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/pitabwire/frame/cache"
)

// Cache is a Redis-backed cache implementation.
type Cache struct {
	client *redis.Client
	maxAge time.Duration
}

const connectionTimeout = 5 * time.Second

// New creates a new Redis cache.
func New(opts ...cache.Option) (cache.RawCache, error) {
	cacheOpts := &cache.Options{
		MaxAge: time.Hour,
	}

	for _, opt := range opts {
		opt(cacheOpts)
	}

	redisOpts, err := redis.ParseURL(cacheOpts.DSN.String())
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(redisOpts)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	err = client.Ping(ctx).Err()
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Cache{
		client: client,
		maxAge: cacheOpts.MaxAge,
	}, nil
}

// Get retrieves an item from the cache.
func (rc *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := rc.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return val, true, nil
}

// Set sets an item in the cache with the specified TTL.
func (rc *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = rc.maxAge
	}

	return rc.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes an item from the cache.
func (rc *Cache) Delete(ctx context.Context, key string) error {
	return rc.client.Del(ctx, key).Err()
}

// Exists checks if a key exists in the cache.
func (rc *Cache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := rc.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Flush clears all items from the cache.
func (rc *Cache) Flush(ctx context.Context) error {
	return rc.client.FlushDB(ctx).Err()
}

// Close closes the Redis connection.
func (rc *Cache) Close() error {
	return rc.client.Close()
}

// Increment atomically increments a counter.
func (rc *Cache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	return rc.client.IncrBy(ctx, key, delta).Result()
}

// Decrement atomically decrements a counter.
func (rc *Cache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return rc.client.DecrBy(ctx, key, delta).Result()
}
