# Monorepo by Default: Monolith or Polylith

Frame assumes a **monorepo-first** layout that can run as:

- **Monolith**: one binary, one service
- **Polylith**: multiple independent binaries from one repo

Both modes share the same shared packages and conventions. The only difference is **how many entrypoints you build**.

## Canonical Layout (Monorepo)

```text
/README.md
/go.mod
/cmd
  /monolith
    main.go
  /users
    main.go
  /billing
    main.go
/apps
  /users
    /cmd
      /users
        main.go
      /users/Dockerfile
    /service
    /config
    /migrations
    /tests
  /billing
    /cmd
      /billing
        main.go
      /billing/Dockerfile
    /service
    /config
    /migrations
    /tests
/pkg
  /shared
  /plugins
  /openapi
/configs
/Dockerfile
```

## Monolith Mode

A single entrypoint composes multiple modules into one binary:

```text
/cmd/monolith/main.go
/apps/users/service
/apps/billing/service
```

All services are wired together in one process. This is ideal for:

- fast local development
- smaller deployments
- shared runtime state

## Polylith Mode (Composable)

Each service has its **own entrypoint** under `/apps/<service>/cmd/<service>`, and a Dockerfile next to it. Shared libraries live in `/pkg`:

```text
/apps/users/cmd/users/main.go
/apps/users/cmd/users/Dockerfile
/apps/billing/cmd/billing/main.go
/apps/billing/cmd/billing/Dockerfile
/pkg/...
```

Each binary is independent, but uses the same `/apps` and `/pkg` packages. This matches the structure used in `service-profile`.

## How to Switch Modes

You do **not** need to restructure anything. You only:

- add or remove `apps/<service>/cmd/<service>/main.go` entrypoints
- build different binaries or Docker images

This makes the repo **composable** by default.

## Recommended Conventions

- `/apps/<service>` is the source of truth for a service
- `/apps/<service>/cmd/<service>` is the polylith entrypoint
- `/cmd/<service>` is the monorepo-level entrypoint (optional)
- `/pkg` is shared infrastructure and cross-cutting plugins
- `/configs` holds environment or YAML configs for all services

## One-Command Scaffold

Frame includes a scaffold tool that creates the monorepo layout with per-service entrypoints and Dockerfiles.

```bash
go run github.com/pitabwire/frame/tools/cmd/frame-init@latest \
  -root . \
  -services users,billing \
  -module your/module
```

This generates:

- `/apps/<service>/cmd/<service>/main.go`
- `/apps/<service>/cmd/<service>/Dockerfile`
- `/cmd/monolith/main.go`
- `/Dockerfile`
- `/pkg` and `/configs` folders

## Example: Polylith Entry Point

```go
package main

import (
	"context"
	"log"

	"github.com/pitabwire/frame"
	"your/module/apps/users/service"
)

func main() {
	ctx, svc := frame.NewService(
		frame.WithName("users"),
		frame.WithHTTPHandler(service.Router()),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
```

## Example: Monolith Entry Point

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/pitabwire/frame"
	"your/module/apps/users/service"
	"your/module/apps/billing/service"
)

func main() {
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)
	billing.RegisterRoutes(mux)

	ctx, svc := frame.NewService(
		frame.WithName("monolith"),
		frame.WithHTTPHandler(mux),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
```
