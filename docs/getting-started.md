# Getting Started with Frame

This guide shows the minimal setup and then builds toward a real service that uses Frame's plugin-based components.

## Install

```bash
go get -u github.com/pitabwire/frame
```

## Default Repo Shape (Monorepo)

Frame defaults to a monorepo layout that supports **monolith** and **polylith** builds without restructuring:

```text
/cmd
  /monolith
  /users
  /billing
/apps
  /users
  /billing
/pkg
  /plugins
  /openapi
/configs
```

See `docs/monorepo.md` for the full structure.

## Minimal HTTP Service (Canonical Pattern)

```go
package main

import (
    "log"
    "net/http"

    "github.com/pitabwire/frame"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Frame says hello"))
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

## Minimal gRPC + HTTP Service

```go
package main

import (
    "log"
    "net/http"

    "github.com/pitabwire/frame"
    "google.golang.org/grpc"
)

func main() {
    grpcServer := grpc.NewServer()

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("HTTP ok"))
    })

    ctx, svc := frame.NewService(
        frame.WithName("hello"),
        frame.WithHTTPHandler(http.DefaultServeMux),
        frame.WithGRPCServer(grpcServer),
        frame.WithGRPCPort(":50051"),
    )

    if err := svc.Run(ctx, ":8080"); err != nil {
        log.Fatal(err)
    }
}
```

## What You Get By Default

- Config loaded from environment (`config.ConfigurationDefault`).
- OpenTelemetry manager (if not disabled).
- Structured logging with optional OTel log handler.
- Resilient HTTP client manager.
- Worker pool and queue manager.
- Events manager (internal event bus).

## Next

- `docs/service.md`
- `docs/configuration.md`
- `docs/server.md`
- `docs/plugins-options.md`
