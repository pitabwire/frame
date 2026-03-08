# HTTP Client

Frame includes a resilient HTTP client with retries, OTel instrumentation, and optional OAuth2 client credentials.

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
- Explicit SPIFFE mTLS for internal service calls, with trust domain resolved from config or context.
- Optional request/response logging.
- OAuth2 client credentials support.
- Automatic `private_key_jwt` client authentication when Frame is configured with Workload API-backed OAuth2 credentials.

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

Startup config provides the trust-domain foundation:

- `WORKLOAD_API_TRUSTED_DOMAIN`

If the service config also sets `OAUTH2_TOKEN_ENDPOINT_AUTH_METHOD=private_key_jwt`
and `OAUTH2_PRIVATE_JWT_KEY.source=workload_api`, the default Frame HTTP client
automatically fetches access tokens using Workload API-backed `private_key_jwt`
client authentication. The assertion is signed with the selected X509-SVID private
key and sent to the discovered OAuth2 token endpoint.

To enable workload API mTLS for a specific downstream call or client, supply a
runtime target. The trust domain is picked up automatically from service config
or a config-bearing context:

```go
cli := client.NewHTTPClient(
    ctx,
    client.WithHTTPWorkloadAPITargetPath("/ns/backend/sa/payments-api"),
)
```

Or provide the full SPIFFE ID directly with `client.WithHTTPWorkloadAPITargetID(...)`.
Trust-domain-wide authorization is also available, but it is explicit:
`client.WithHTTPWorkloadAPITrustDomain("example.org")`.

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
## Best Practices

- Use a single client per service.
- Avoid logging response bodies in production.
- Set timeouts per upstream service type.
