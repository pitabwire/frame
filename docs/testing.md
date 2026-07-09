# Testing

Frame includes test utilities in `frametests` and `tests` to support integration and component tests.

## What Exists

- `frametests/driver.go`: test server drivers
- `frametests/testsuite.go`: reusable test suites
- `frametests/deps`: test dependencies (Postgres, NATS, Hydra, Keto, Valkey)

## Strategy

- Use `frametests` utilities to spin up test dependencies.
- Exercise `Service.Run` with a test driver or ephemeral ports.
- Use `config.ConfigurationDefault` with env overrides.

## Example Pattern

```go
ctx, svc := frame.NewService(
    frame.WithName("test"),
    frame.WithHTTPHandler(http.DefaultServeMux),
    frametests.WithNoopDriver(), // or WithHTTPTestDriver() when you need httptest
)
defer svc.Stop(ctx)

// Test drivers return from ListenAndServe once ready (non-blocking).
// Run completes after startups finish — no goroutine required.
err := svc.Run(ctx, "")
require.NoError(t, err)
```

## Tips

- Prefer integration tests for queue, cache, and datastore.
- Use `mem://` drivers for fast unit tests.
- Use `frametests.WithNoopDriver()` when HTTP is not under test; use `WithHTTPTestDriver()` when you need a live `httptest.Server`.
- Do **not** wrap `svc.Run` in a goroutine for test drivers — that is only needed for production-style drivers that block in `ListenAndServe`.
