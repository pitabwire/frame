# Frame Documentation

A fast, extensible Golang framework with a clean plugin-based architecture.

Frame is a production-focused framework for building HTTP and gRPC services with strong runtime management, modular components, and convention-driven ergonomics. Frame integrates Go Cloud for pluggable infrastructure, provides first-class support for queues, caching, datastore, telemetry, security, localization, and worker pools, and keeps the core service lifecycle explicit and testable.

## Quick Start (5 lines)

```go
package main

import (
    "context"
    "net/http"

    "github.com/pitabwire/frame"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello from Frame"))
    })

    _, svc := frame.NewService(
        frame.WithName("hello"),
        frame.WithHTTPHandler(http.DefaultServeMux),
    )
    _ = svc.Run(context.Background(), ":8080")
}
```

## Documentation Map

Start here:
- `docs/getting-started.md`
- `docs/architecture.md`
- `docs/service.md`

Core runtime and server:
- `docs/server.md`
- `docs/configuration.md`
- `docs/logging.md`
- `docs/telemetry.md`
- `docs/profiler.md`
- `docs/tls.md`

Data and infrastructure:
- `docs/datastore.md`
- `docs/cache.md`
- `docs/queue.md`
- `docs/events.md`
- `docs/workerpool.md`
- `docs/ratelimiter.md`
- `docs/client.md`

Security and identity:
- `docs/security-authentication.md`
- `docs/security-authorization.md`
- `docs/security-interceptors.md`

Localization and utilities:
- `docs/localization.md`
- `docs/data-utilities.md`
- `docs/testing.md`

## What Makes Frame Different

- Modular, convention-driven components with a small core.
- Go Cloud integration for multi-provider portability.
- Pluggable runtime with explicit startup and shutdown hooks.
- Strong defaults for telemetry, logging, and resilience.
- Designed for production-grade services in Go.

## Versioning

Frame exposes build metadata at runtime via `frame.Service.Run`, including repository, version, commit, and build date (see `version/version.go`).
