# HTTP Client

Frame includes a resilient HTTP client with retries, circuit breaking, OTel instrumentation, and optional OAuth2 client credentials.

## Quick Start

```go
clientMgr := svc.HTTPClientManager()
cli := clientMgr.Client(ctx)

req, _ := http.NewRequest("GET", "https://example.com", nil)
resp, err := cli.Do(req)
```

## Features

- Automatic OpenTelemetry instrumentation.
- Retry policy on transient failures.
- Per-host circuit breaker using `gobreaker`.
- Optional request/response logging.
- OAuth2 client credentials support.

## Configure

```go
_, svc := frame.NewService(
    frame.WithHTTPClient(
        client.WithHTTPTimeout(10*time.Second),
        client.WithHTTPRetryPolicy(&client.RetryPolicy{MaxAttempts: 5}),
        client.WithHTTPTraceRequests(),
    ),
)
```

## REST Invoker

The `client.Manager` also provides helpers to call JSON endpoints:

```go
resp, err := clientMgr.Invoke(ctx, "POST", "https://api.example.com", payload, nil)
if err == nil {
    var out MyResponse
    _ = resp.Decode(ctx, &out)
}
```

## Default Behaviors

- Timeout: 30s
- Retry attempts: 3
- Backoff: quadratic
- Circuit breaker max entries: 1024

## Best Practices

- Use a single client per service.
- Avoid logging response bodies in production.
- Set timeouts per upstream service type.
