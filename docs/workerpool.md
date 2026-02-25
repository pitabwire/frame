# Worker Pool

Frame includes a worker pool backed by `ants` with retry scheduling and a typed job API.

## Quick Start

```go
job := workerpool.NewJob("job-1", func(ctx context.Context, pipe workerpool.JobResultPipe[string]) error {
    return pipe.WriteResult(ctx, "ok")
})

_ = frame.SubmitJob(ctx, svc, job)
result, _ := job.ReadResult(ctx)
```

## Configure

Frame configures a worker pool using `ConfigurationWorkerPool`:

- CPU factor
- pool capacity
- pool count
- expiry duration

You can override with `WithWorkerPoolOptions` and `workerpool.Option` values:

- `WithPoolCount`
- `WithSinglePoolCapacity`
- `WithConcurrency`
- `WithPoolExpiryDuration`
- `WithPoolNonblocking`
- `WithPoolPreAlloc`
- `WithPoolPanicHandler`
- `WithPoolLogger`
- `WithPoolDisablePurge`

## Retry Behavior

Jobs support retries and exponential backoff. The manager schedules retries via an internal queue. Maximum retry runs and backoff are enforced.

## API Highlights

- `workerpool.NewManager(ctx, cfg, stopErr, opts...)`
- `Manager.GetPool()`
- `workerpool.SubmitJob(ctx, manager, job)`
- `workerpool.NewJob(id, func)`

## Best Practices

- Keep job functions small and idempotent.
- Use typed results to avoid serialization overhead.
- Monitor retry rates to detect systemic failures.
