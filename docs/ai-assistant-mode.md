# AI Assistant Mode (Draft Spec)

AI Assistant Mode makes Frame **AI-operable**: services can be introspected, generated, and extended by agents using deterministic patterns and machine-readable surfaces.

This spec defines the initial surface area for AI tooling and the runtime rules that make Frame safe to generate.

## Goals

- Deterministic patterns for codegen.
- Machine-readable service metadata.
- Safe introspection endpoints for agents.
- Predictable extension model (Options = Plugins).

## Canonical Bootstrap Pattern (Required)

All AI-generated code must use this pattern:

```go
ctx, svc := frame.NewService(
    frame.WithName("my-service"),
    frame.WithHTTPHandler(router),
    // other options...
)

if err := svc.Run(ctx, ":8080"); err != nil {
    log.Fatal(err)
}
```

## Runtime Modes: Monolith vs Polylith

Frame supports two deployment modes configurable via **env** and **YAML**.

### Monolith Mode

Single binary running **multiple services** (multiple handlers, queues, plugins) as one process.

### Polylith Mode

Multiple **independent binaries** from one codebase (shared components, separate `apps/<service>/cmd/main.go`).

### Configuration Contract

Environment variables:

- `FRAME_RUNTIME_MODE=monolith|polylith`
- `FRAME_SERVICE_ID=devices`
- `FRAME_SERVICE_GROUP=profile`

YAML:

```yaml
runtime_mode: polylith
service_id: devices
service_group: profile
```

## Introspection Endpoints (JSON)

Enabled via `WithDebugEndpoints()` or `FRAME_DEBUG_ENDPOINTS=true`.

- `GET /debug/frame/config`
- `GET /debug/frame/plugins`
- `GET /debug/frame/routes`
- `GET /debug/frame/queues`
- `GET /debug/frame/health`

All responses must be deterministic and safe to parse by agents.

## CLI (v0.1)

```
frame g service <name>
frame g http <route> <method>
frame g queue publisher <ref> <url>
frame g queue subscriber <ref> <url> <handler>
frame build blueprint.yaml
```

## AI Contract

- Always use the canonical bootstrap pattern.
- Options are plugins; plugins are `WithXxx` functions returning `frame.Option`.
- Handlers live in `/internal/handlers`.
- Plugins live in `/internal/plugins`.
- Avoid globals; use context + service managers.
