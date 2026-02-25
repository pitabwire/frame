# Rate Limiter

Frame includes a cache-backed fixed-window rate limiter for HTTP or internal use.

## Window Limiter

The `WindowLimiter` uses `RawCache.Increment` and per-key TTLs.

```go
raw, _ := svc.GetRawCache("redis")
limiter, _ := ratelimiter.NewWindowLimiter(raw, ratelimiter.DefaultWindowConfig())

if !limiter.Allow(ctx, "user:123") {
    // reject
}
```

## Configuration

```go
cfg := &ratelimiter.WindowConfig{
    WindowDuration: time.Minute,
    MaxPerWindow: 100,
    KeyPrefix: "api",
    FailOpen: false,
}
```

## Cache Requirements

The cache backend must support per-key TTLs. If not, `ErrCacheDoesNotSupportPerKeyTTL` is returned.

## Best Practices

- Use a shared cache (Redis/Valkey) for multi-instance rate limiting.
- Use a stable key naming scheme (`tenant:user`, `api_key`).
- Prefer fail-closed for sensitive endpoints.
