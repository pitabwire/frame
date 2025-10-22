package valkey

import (
	"context"
	"net/url"
	"time"

	"github.com/pitabwire/frame/cache"
	"github.com/valkey-io/valkey-go"
)

// Options contains configuration for Valkey cache.
type Options struct {
	Addr     string
	Password string
	DB       int
}

// Cache is a Valkey-backed cache implementation using the official Valkey client.
type Cache struct {
	client valkey.Client
}

const connectionTimeout = 5 * time.Second

// New creates a new Valkey cache.
func New(opts Options) (cache.RawCache, error) {
	// Parse address to handle redis:// scheme
	addr := opts.Addr
	if parsedURL, err := url.Parse(opts.Addr); err == nil && parsedURL.Scheme == "redis" {
		addr = parsedURL.Host
	}

	// Create Valkey client options
	clientOpts := valkey.ClientOption{
		InitAddress: []string{addr},
	}

	// Add password if provided
	if opts.Password != "" {
		clientOpts.Password = opts.Password
	}

	// Add DB selection if provided (SelectDB option)
	if opts.DB != 0 {
		clientOpts.SelectDB = opts.DB
	}

	// Create the client
	client, err := valkey.NewClient(clientOpts)
	if err != nil {
		return nil, err
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	if pingErr := client.Do(ctx, client.B().Ping().Build()).Error(); pingErr != nil {
		client.Close()
		return nil, pingErr
	}

	return &Cache{
		client: client,
	}, nil
}

// Get retrieves an item from the cache.
func (vc *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	cmd := vc.client.B().Get().Key(key).Build()
	resp := vc.client.Do(ctx, cmd)

	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	val, err := resp.AsBytes()
	if err != nil {
		return nil, false, err
	}

	return val, true, nil
}

// Set sets an item in the cache with the specified TTL.
func (vc *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	var cmd valkey.Completed

	if ttl > 0 {
		// Valkey Ex() expects seconds, not duration
		seconds := int64(ttl.Seconds())
		if seconds == 0 {
			seconds = 1 // Minimum 1 second for sub-second durations
		}
		cmd = vc.client.B().Set().Key(key).Value(valkey.BinaryString(value)).ExSeconds(seconds).Build()
	} else {
		cmd = vc.client.B().Set().Key(key).Value(valkey.BinaryString(value)).Build()
	}

	return vc.client.Do(ctx, cmd).Error()
}

// Delete removes an item from the cache.
func (vc *Cache) Delete(ctx context.Context, key string) error {
	cmd := vc.client.B().Del().Key(key).Build()
	return vc.client.Do(ctx, cmd).Error()
}

// Exists checks if a key exists in the cache.
func (vc *Cache) Exists(ctx context.Context, key string) (bool, error) {
	cmd := vc.client.B().Exists().Key(key).Build()
	resp := vc.client.Do(ctx, cmd)

	if err := resp.Error(); err != nil {
		return false, err
	}

	count, err := resp.AsInt64()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Flush clears all items from the cache.
func (vc *Cache) Flush(ctx context.Context) error {
	cmd := vc.client.B().Flushdb().Build()
	return vc.client.Do(ctx, cmd).Error()
}

// Close closes the Valkey connection.
func (vc *Cache) Close() error {
	vc.client.Close()
	return nil
}

// Increment atomically increments a counter.
func (vc *Cache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	cmd := vc.client.B().Incrby().Key(key).Increment(delta).Build()
	resp := vc.client.Do(ctx, cmd)

	if err := resp.Error(); err != nil {
		return 0, err
	}

	return resp.AsInt64()
}

// Decrement atomically decrements a counter.
func (vc *Cache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	cmd := vc.client.B().Decrby().Key(key).Decrement(delta).Build()
	resp := vc.client.Do(ctx, cmd)

	if err := resp.Error(); err != nil {
		return 0, err
	}

	return resp.AsInt64()
}
