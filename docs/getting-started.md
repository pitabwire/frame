# Getting Started with Frame

This guide shows the minimal setup and then builds toward a real service that uses Frame's plugin-based components.

## Install

```bash
go get -u github.com/pitabwire/frame
```

## Minimal HTTP Service

```go
package main

import (
    "context"
    "net/http"

    "github.com/pitabwire/frame"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Frame says hello"))
    })

    _, svc := frame.NewService(
        frame.WithName("hello"),
        frame.WithHTTPHandler(http.DefaultServeMux),
    )

    _ = svc.Run(context.Background(), ":8080")
}
```

## Minimal gRPC + HTTP Service

```go
package main

import (
    "context"
    "net/http"

    "github.com/pitabwire/frame"
    "google.golang.org/grpc"
)

func main() {
    grpcServer := grpc.NewServer()

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("HTTP ok"))
    })

    _, svc := frame.NewService(
        frame.WithName("hello"),
        frame.WithHTTPHandler(http.DefaultServeMux),
        frame.WithGRPCServer(grpcServer),
        frame.WithGRPCPort(":50051"),
    )

    _ = svc.Run(context.Background(), ":8080")
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
