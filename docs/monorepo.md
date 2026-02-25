# Monorepo by Default: Monolith or Polylith

Frame uses a **monorepo-first** layout. You can run the same codebase as:

- **Monolith**: one Frame service, one mux, many routes.
- **Polylith**: many independent binaries, each service deployed separately.

## Canonical Layout

```text
/README.md
/go.mod
/cmd
  /users/main.go
  /billing/main.go
/apps
  /users
    /cmd/main.go
    /service/routes.go
    /queues
    /config
    /migrations
    /tests
    /Dockerfile
  /billing
    /cmd/main.go
    /service/routes.go
    /queues
    /config
    /migrations
    /tests
    /Dockerfile
/pkg
  /plugins
  /openapi
  /shared
/configs
/Dockerfile
```

## Monolith Mode (Single Service)

Monolith means **one `frame.Service` + one `http.ServeMux`**. Multiple app routes are mounted into that one mux.

Example shape:

```go
mux := http.NewServeMux()
users.RegisterRoutes(mux)
billing.RegisterRoutes(mux)

ctx, svc := frame.NewService(
    frame.WithName("monolith"),
    frame.WithHTTPHandler(mux),
)

if err := svc.Run(ctx, ":8080"); err != nil { ... }
```

## Polylith Mode (Independent Binaries)

Each app has its own binary entrypoint and Dockerfile:

- `apps/<service>/cmd/main.go`
- `apps/<service>/Dockerfile`

The repo-level `cmd/<service>/main.go` gives a consistent top-level build/run entrypoint.

## One-Command Scaffold

```bash
go run github.com/pitabwire/frame/cmd/frame@latest init \
  -root . \
  -services users,billing \
  -module your/module
```

`-module` is optional. If omitted, Frame tries `go.mod`, then falls back to `example.com/project`.

Generated artifacts:

- `apps/<service>/cmd/main.go`
- `apps/<service>/service/routes.go`
- `apps/<service>/Dockerfile`
- `cmd/<service>/main.go`
- `Dockerfile`
- `pkg` and `configs`

## Build Patterns

Polylith binary:

```bash
go build ./apps/users/cmd
```

Monorepo-level binary for one app:

```bash
go build ./cmd/users
```

Single-binary monolith is generated from blueprints in monolith mode (`frame build`), producing `cmd/main.go` that composes all routes into one mux.

## Docker Build Patterns

Monorepo-level Dockerfile (`/Dockerfile`) uses a multi-stage builder and copies:

- `/apps`
- `/pkg`
- `/cmd`

Build an app via top-level cmd entrypoint:

```bash
docker build -t users-service --build-arg APP=users .
```

Per-service Dockerfile (`/apps/<service>/Dockerfile`) copies:

- `/apps/<service>`
- `/pkg`

Build a single polylith app:

```bash
docker build -t users-service -f apps/users/Dockerfile .
```
