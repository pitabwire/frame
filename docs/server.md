# HTTP and gRPC Server

Frame provides an HTTP server with optional gRPC sidecar support. The server layer is configured via `Service` options and environment-based config interfaces.

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

## gRPC Server

```go
grpcServer := grpc.NewServer()

ctx, svc := frame.NewService(
    frame.WithHTTPHandler(http.DefaultServeMux),
    frame.WithGRPCServer(grpcServer),
    frame.WithGRPCPort(":50051"),
)

_ = svc.Run(ctx, ":8080")
```

Optional:
- `WithEnableGRPCServerReflection()`
- `WithGRPCServerListener(listener net.Listener)`

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

## Default HTTP Timeouts

These defaults are hard-coded in `service.go`:

- Read timeout: 5s
- Write timeout: 10s
- Idle timeout: 90s

Adjust by supplying a custom driver.
