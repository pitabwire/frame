# Cache

Frame provides a cache manager with multiple backends and typed cache wrappers.

## Concepts

- `RawCache`: low-level interface with byte values.
- `Cache[K,V]`: typed cache built on top of raw cache.
- `Manager`: holds named caches.

## Quick Start (In-Memory)

```go
_, svc := frame.NewService(
    frame.WithInMemoryCache("local"),
)

raw, _ := svc.GetRawCache("local")
_ = raw.Set(ctx, "key", []byte("value"), time.Minute)
```

## Typed Cache

```go
cacheMgr := svc.CacheManager()
users, ok := cache.GetCache[string, User](cacheMgr, "local", func(id string) string { return id })
if ok {
    _ = users.Set(ctx, "u1", User{ID: "u1"}, time.Minute)
}
```

## Backends

- In-memory: `cache.NewInMemoryCache()`
- Redis: `cache/redis.New` (DSN via `cache.WithDSN`)
- Valkey: `cache/valkey.New`
- JetStream KV: `cache/jetstreamkv.New`

## Manager API

- `cache.NewManager()`
- `AddCache(name string, raw RawCache)`
- `GetRawCache(name string)`
- `RemoveCache(name string)`
- `Close()`

## Rate Limiting Integration

The cache interface supports `Increment` and `Expire`, which enables cache-backed rate limiting (see `docs/ratelimiter.md`).

## Best Practices

- Prefer in-memory caches for per-instance data.
- Use Redis or JetStream KV for shared caches.
- Set explicit TTLs for large objects.
