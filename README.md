# Frame

[![Build Status](https://github.com/pitabwire/frame/actions/workflows/run_tests.yml/badge.svg?branch=main)](https://github.com/pitabwire/frame/actions/workflows/run_tests.yml)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/pitabwire/frame)

A fast, extensible Golang framework with a clean plugin-based architecture.

Frame helps you spin up HTTP and gRPC services with minimal boilerplate while keeping strong runtime management, observability, and portable infrastructure via Go Cloud.

## Features

- HTTP and gRPC servers with built-in lifecycle management
- Datastore setup using GORM with migrations and multi-tenancy
- Queue publish/subscribe (Go Cloud drivers: `mem://`, `nats://`, etc.)
- Cache manager with multiple backends
- OpenTelemetry tracing, metrics, and logs
- OAuth2/JWT authentication and authorization adapters
- Worker pool for background tasks
- Localization utilities

## Install

```bash
go get -u github.com/pitabwire/frame
```

## Minimal Example

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

## Documentation

- Start here: `docs/index.md`
- Live site: https://pitabwire.github.io/frame/

## Docs Site (MkDocs)

```bash
pip install mkdocs mkdocs-material
mkdocs serve
```

## Development

To run tests, start the Docker Compose file in `./tests`, then run:

```bash
go test -json -cover ./...
```

## Contributing

If Frame helped you, please consider starring the repo and sharing it.

We’re actively looking for contributions that make Frame easier to use and more productive for Go teams. Examples:

- Improve onboarding guides or add real-world examples
- Add new Go Cloud drivers (queue, cache, datastore)
- Add middleware, interceptors, or CLI tooling
- Expand testing utilities or reference architectures

AI-assisted contributions are welcome. If you use AI tools, please verify behavior locally and include tests where relevant.

Open an issue or PR with your idea — even small improvements make a big difference.
