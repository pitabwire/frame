# HTTP Server

Frame provides an HTTP server with configurable lifecycle and transport limits. The server layer is configured via `Service` options and environment-based config interfaces.

## HTTP Server Basics

```go
http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("ok"))
})

ctx, svc := frame.NewService(
    frame.WithName("api"),
    frame.WithHTTPHandler(http.DefaultServeMux),
)

_ = svc.Run(ctx, ":8080")
```

### Middleware

```go
logging := func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s", r.Method, r.URL.Path)
        next.ServeHTTP(w, r)
    })
}

ctx, svc := frame.NewService(
    frame.WithHTTPHandler(http.DefaultServeMux),
    frame.WithHTTPMiddleware(logging),
)

_ = svc.Run(ctx, ":8080")
```

### Health Checks

- Default path: `/healthz`
- Register custom checks using `AddHealthCheck`.

## HTTP/2 Support

Frame configures HTTP/2 support automatically:

- h2c (HTTP/2 without TLS) for non-TLS HTTP
- standard HTTP/2 for TLS

## TLS

TLS is enabled by configuration. The server selects TLS based on `ConfigurationTLS` (see `docs/tls.md`).

## Server Driver

Frame uses a driver abstraction compatible with Go Cloud server drivers:

```go
type ServerDriver interface {
    driver.Server
    driver.TLSServer
}
```

You can inject a custom driver with `WithDriver` for testing or custom transport.

## Default HTTP Limits

These defaults are configurable via `config.ConfigurationHTTPServer` and environment variables:

- `HTTP_SERVER_READ_TIMEOUT=30s`
- `HTTP_SERVER_READ_HEADER_TIMEOUT=5s`
- `HTTP_SERVER_WRITE_TIMEOUT=30s`
- `HTTP_SERVER_IDLE_TIMEOUT=90s`
- `HTTP_SERVER_SHUTDOWN_TIMEOUT=15s`
- `HTTP_SERVER_MAX_HEADER_KB=1024`
