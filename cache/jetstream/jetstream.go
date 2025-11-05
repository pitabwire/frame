package jetstream

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/pitabwire/frame/cache"
)

// Cache is a JetStream-backed cache implementation using the NATS KeyValue store.
type Cache struct {
	conn   *nats.Conn
	client nats.KeyValue
	maxAge time.Duration
}

// New creates a new Valkey cache.
func New(opts ...cache.Option) (cache.RawCache, error) {
	cacheOpts := &cache.Options{
		Name:   "default",
		MaxAge: time.Hour,
	}

	for _, opt := range opts {
		opt(cacheOpts)
	}

	natsConn, err := nats.Connect(cacheOpts.DSN.String())
	if err != nil {
		return nil, err
	}

	js, err := natsConn.JetStream()
	if err != nil {
		return nil, err
	}
	// Create the client
	kvCfg := &nats.KeyValueConfig{
		Bucket: cacheOpts.Name,
		TTL:    cacheOpts.MaxAge, // expiry for entries
		// History, MaxBytes, etc. may also be set
	}

	client, err := js.CreateKeyValue(kvCfg)
	if err != nil {

		var apiErr *nats.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode == nats.JSErrCodeStreamNameInUse {

			// If the bucket already exists, just get a handle to it.
			client, err = js.KeyValue(cacheOpts.Name)
			if err != nil {
				return nil, err
			}
		} else {
			// Another error occurred during creation.
			return nil, err
		}
	}

	if _, err = client.Status(); err != nil {
		return nil, err
	}

	return &Cache{
		conn:   natsConn,
		client: client,
		maxAge: cacheOpts.MaxAge,
	}, nil
}

// Get retrieves an item from the cache.
func (vc *Cache) Get(_ context.Context, key string) ([]byte, bool, error) {
	resp, err := vc.client.Get(key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return resp.Value(), true, nil
}

// Set sets an item in the cache with the specified TTL.
func (vc *Cache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	_, err := vc.client.Put(key, value)
	if err != nil {
		return err
	}
	return nil
}

// Delete removes an item from the cache.
func (vc *Cache) Delete(_ context.Context, key string) error {
	return vc.client.Delete(key)
}

// Exists checks if a key exists in the cache.
func (vc *Cache) Exists(_ context.Context, key string) (bool, error) {
	_, err := vc.client.Get(key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Flush clears all items from the cache.
func (vc *Cache) Flush(_ context.Context) error {
	keys, err := vc.client.Keys()
	if err != nil {
		return err
	}

	for _, key := range keys {
		err = vc.client.Delete(key)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close closes the Valkey connection.
func (vc *Cache) Close() error {
	vc.conn.Close()
	return nil
}

// isRevisionConflict returns true if the error represents an optimistic concurrency conflict.
func (vc *Cache) isRevisionConflict(err error) bool {
	var apiErr *nats.APIError
	if errors.As(err, &apiErr) {
		// According to NATS API codes, code 10071 is wrong last sequence (conflict)
		return apiErr.ErrorCode == nats.JSErrCodeStreamWrongLastSequence
	}
	return false
}

// Increment atomically increments a counter.
func (vc *Cache) Increment(_ context.Context, key string, delta int64) (int64, error) {
	for {
		entry, err := vc.client.Get(key)
		if errors.Is(err, nats.ErrKeyNotFound) {
			// Create key if not exists
			val := strconv.FormatInt(delta, 10)
			rev, createErr := vc.client.Create(key, []byte(val))
			if errors.Is(createErr, nats.ErrKeyExists) {
				continue // race, retry
			}
			if createErr != nil {
				return 0, createErr
			}
			_ = rev
			return delta, nil
		} else if err != nil {
			return 0, err
		}

		currentVal, err := strconv.ParseInt(string(entry.Value()), 10, 64)
		if err != nil {
			return 0, err
		}
		newVal := currentVal + delta

		newRev, err := vc.client.Update(key, []byte(strconv.FormatInt(newVal, 10)), entry.Revision())
		if vc.isRevisionConflict(err) {
			continue // conflict, retry
		}
		if err != nil {
			return 0, err
		}
		_ = newRev
		return newVal, nil
	}
}

// Decrement atomically decrements a counter.
func (vc *Cache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return vc.Increment(ctx, key, -delta)
}
