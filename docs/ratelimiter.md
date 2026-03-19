# Rate Limiter

`frame/ratelimiter` now provides four different admission-control primitives.
They are intentionally different because production overload does not show up in
just one way.

If you only remember one rule, remember this one:

> Pick the limiter that matches the resource that fails first.

If you pick the wrong limiter, the code may still look correct while the system
fails under load.

## Quick Choice Guide

| If the real problem is... | Use | Why |
|---|---|---|
| One tenant, IP, or API key can send too many requests in a time window | `WindowLimiter` | Fixed-window quota per shared key |
| Same as above, but the cache increment itself becomes a bottleneck at high volume | `LeasedWindowLimiter` | Same quota semantics, fewer remote cache operations |
| Too many expensive operations run at the same time in one process | `ConcurrencyLimiter` | Caps local in-flight work |
| Backlog in a queue, outbox, or worker pipeline is already too high | `QueueDepthLimiter` | Stops admitting more work until backlog is healthy |

## Do Not Confuse These Limiters

### `WindowLimiter`

Use `WindowLimiter` when you want a straightforward, shared, distributed budget
per key and the cache backend can comfortably absorb one atomic increment per
request.

Correct use:

- per-tenant API request budgets
- per-user or per-IP HTTP protection
- moderate-volume internal service quotas

Wrong use:

- protecting CPU-heavy work that fails because too many tasks run at once
- protecting queue backlog growth
- very high-volume hot keys where cache traffic becomes expensive

Example:

```go
raw, err := svc.GetRawCache("redis")
if err != nil {
	return err
}

limiter, err := ratelimiter.NewWindowLimiter(raw, &ratelimiter.WindowConfig{
	WindowDuration: time.Minute,
	MaxPerWindow:   500,
	KeyPrefix:      "tenant:api",
	FailOpen:       false,
})
if err != nil {
	return err
}

if !limiter.Allow(ctx, tenantID) {
	return connect.NewError(connect.CodeResourceExhausted, errors.New("tenant request budget exceeded"))
}
```

### `LeasedWindowLimiter`

Use `LeasedWindowLimiter` when the semantics of a fixed-window quota are still
correct, but a remote increment on every request would create a hot-key or
cache-throughput problem.

The limiter works by reserving quota from the shared cache in chunks, then
serving several local `Allow` calls from that reservation before touching the
cache again.

Correct use:

- very hot per-tenant ingest limits
- webhook ingress or event ingest with sustained high volume
- cases where you still want a distributed fixed-window budget

Wrong use:

- exact per-request observability of every increment at the cache layer
- low-volume endpoints where plain `WindowLimiter` is simpler
- concurrency protection for expensive local work

Guidance:

- Use the default reservation size unless profiling shows it is wrong.
- Larger reservations reduce cache traffic but make each process hold more local
  quota at once.
- Smaller reservations increase cache traffic but reduce local over-reservation.

Example:

```go
raw, err := svc.GetRawCache("redis")
if err != nil {
	return err
}

limiter, err := ratelimiter.NewLeasedWindowLimiter(raw, &ratelimiter.WindowConfig{
	WindowDuration:  time.Minute,
	MaxPerWindow:    1_000_000,
	KeyPrefix:       "tenant:event-ingest",
	FailOpen:        false,
	ReservationSize: 1024,
})
if err != nil {
	return err
}

if !limiter.Allow(ctx, tenantID) {
	return connect.NewError(connect.CodeResourceExhausted, errors.New("tenant ingest budget exceeded"))
}
```

### `ConcurrencyLimiter`

Use `ConcurrencyLimiter` when the resource you are protecting is local and
finite. This is about simultaneous work, not requests per minute.

Correct use:

- capping in-flight connector calls in a worker
- bounding CPU-heavy transforms
- limiting database-heavy handlers in one process
- preventing one process from fan-out exploding into thousands of active goroutines

Wrong use:

- global tenant fairness across many replicas
- distributed rate limiting across a fleet
- backlog admission at producer boundaries

Important behavior:

- The limit is process-local.
- Ten pods with limit `100` each can run about `1000` concurrent operations.
- `TryAcquire` is for fail-fast behavior.
- `Acquire` is for bounded waiting and should usually be paired with a context
  deadline.

Example:

```go
connectorLimiter, err := ratelimiter.NewConcurrencyLimiter(128)
if err != nil {
	return err
}

permit, ok := connectorLimiter.TryAcquire()
if !ok {
	return ratelimiter.ErrConcurrencyLimitReached
}
defer permit.Release()

return connector.Execute(ctx, req)
```

If the caller should wait briefly for capacity:

```go
connectorLimiter, err := ratelimiter.NewConcurrencyLimiter(128)
if err != nil {
	return err
}

ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
defer cancel()

permit, err := connectorLimiter.Acquire(ctx)
if err != nil {
	return err
}
defer permit.Release()

return connector.Execute(ctx, req)
```

### `QueueDepthLimiter`

Use `QueueDepthLimiter` when the best overload signal is not raw request rate,
but backlog in a downstream work queue, outbox, retry queue, or scheduler
pipeline.

This limiter is an admission controller. It does not smooth traffic. It simply
decides whether more work should be accepted right now.

Correct use:

- stopping event ingest when outbox backlog is unsafe
- pausing enqueue when worker queue depth is already too high
- preventing retry storms from making backlog worse

Wrong use:

- tenant fairness
- abuse protection
- shaping requests into a stable average rate

Important behavior:

- `RejectAtDepth` closes admission.
- `ResumeAtDepth` reopens admission.
- `ResumeAtDepth` must be lower than `RejectAtDepth`.
- That gap is deliberate hysteresis so the system does not flap between open and
  closed every few milliseconds.
- `RefreshInterval` exists because depth lookups are often remote calls and
  should not happen on every request.

Example:

```go
depthLimiter, err := ratelimiter.NewQueueDepthLimiter(
	func(ctx context.Context) (int64, error) {
		return queue.Pending(ctx, "workflow-events")
	},
	ratelimiter.QueueDepthConfig{
		RejectAtDepth:  250_000,
		ResumeAtDepth:  150_000,
		RefreshInterval: 500 * time.Millisecond,
		FailOpen:       false,
	},
)
if err != nil {
	return err
}

if !depthLimiter.Allow(ctx) {
	return connect.NewError(connect.CodeResourceExhausted, errors.New("event backlog too high"))
}
```

## Recommended Layering

Many real systems need more than one limiter.

For example, a high-throughput event ingest service commonly needs:

1. `LeasedWindowLimiter` at ingress for tenant fairness.
2. `QueueDepthLimiter` before enqueue so backlog cannot grow without bound.
3. `ConcurrencyLimiter` in workers or connector executors so expensive local
   work does not saturate the process.

These are complementary, not redundant.

## Failure-Mode Guidance

Choose `FailOpen` vs `FailOpen=false` deliberately:

- Fail open when temporary inability to measure should not stop user traffic.
- Fail closed when the protected downstream system is sensitive enough that
  admitting work without measurement is too risky.

Examples:

- Public webhook ingest commonly fails open for request-rate checks but may fail
  closed for severe backlog admission.
- Financially sensitive connector execution often fails closed on local
  concurrency or dependency health checks.

## What To Avoid

- Do not use only a request-rate limiter when the real failure mode is queue
  buildup.
- Do not use only a queue-depth limiter when one tenant can starve everyone
  else.
- Do not use only a concurrency limiter when many replicas together can still
  overload a shared downstream dependency.

Production systems usually need a combination, not a single universal limiter.
