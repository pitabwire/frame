# Frame Documentation

A Go framework for building vendor-agnostic cloud-native services with HTTP, gRPC, and Connect RPC.

Frame is a production-focused framework for building cloud-native services that stay vendor-agnostic. It supports HTTP, gRPC, and Connect RPC out of the box, with strong runtime management, modular components, and convention-driven ergonomics. Frame integrates Go Cloud for pluggable infrastructure, provides first-class support for queues, caching, datastore, telemetry, security, localization, and worker pools, and keeps the core service lifecycle explicit and testable.

## Quick Start (Canonical Pattern)

```go
package main

import (
    "log"
    "net/http"

    "github.com/pitabwire/frame"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello from Frame"))
    })

    ctx, svc := frame.NewService(
        frame.WithName("hello"),
        frame.WithHTTPHandler(http.DefaultServeMux),
    )

    if err := svc.Run(ctx, ":8080"); err != nil {
        log.Fatal(err)
    }
}
```

## Documentation Map

Start here:
- `docs/getting-started.md`
- `docs/architecture.md`
- `docs/service.md`
- `docs/plugins-options.md`
- `docs/ai-assistants.md`
- `docs/examples.md`

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

- **Cloud-native and vendor-agnostic** — Go Cloud integration means no lock-in to a single provider.
- **HTTP, gRPC, and Connect RPC** — first-class support for all three protocols in a single service.
- Modular, convention-driven components with a small core.
- Pluggable runtime with explicit startup and shutdown hooks.
- Strong defaults for telemetry, logging, and resilience.
- Designed for production-grade services in Go.

## Versioning

Frame exposes build metadata at runtime via `frame.Service.Run`, including repository, version, commit, and build date (see `version/version.go`).
