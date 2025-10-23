package redis

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/pitabwire/frame/cache"
)

// Options contains configuration for Redis cache.
type Options struct {
	Addr     string
	Password string
	DB       int
}

// Cache is a Redis-backed cache implementation.
type Cache struct {
	client *redis.Client
}

const connectionTimeout = 5 * time.Second

// New creates a new Redis cache.
func New(opts Options) (cache.RawCache, error) {
	// Parse address to handle redis:// scheme
	addr := opts.Addr
	if parsedURL, err := url.Parse(opts.Addr); err == nil && parsedURL.Scheme == "redis" {
		addr = parsedURL.Host
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: opts.Password,
		DB:       opts.DB,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Cache{
		client: client,
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
