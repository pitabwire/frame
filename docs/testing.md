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
)

// call svc.Run in a goroutine and use a test driver if needed
```

## Tips

- Prefer integration tests for queue, cache, and datastore.
- Use `mem://` drivers for fast unit tests.
