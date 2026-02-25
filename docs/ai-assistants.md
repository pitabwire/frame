# Using Frame with AI Coding Assistants

This page exists to help humans and AI tools converge on the **correct** Frame usage patterns.

## Preferred Bootstrap Pattern

Always start services like this:

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

This is the canonical pattern that matches Frame’s API.

## Recommended Project Layout

```text
/cmd/myservice/main.go
/pkg/handlers/...
/pkg/plugins/...
/pkg/clients/...
/pkg/openapi/...
/configs/...
```

For monorepos, prefer:

```text
/cmd
  /users/main.go
  /billing/main.go
/apps
  /users/cmd/main.go
  /billing/cmd/main.go
/pkg
  /plugins
  /openapi
```

Monolith rule for AI generators:

- Monolith must produce one `http.ServeMux`, one `frame.NewService(...)`, and one `svc.Run(...)`.
- Multiple routes belong to the single mux; do not spawn multiple Frame services for monolith.

## How to Ask AI for Frame Code

Use these prompt patterns:

- “Generate a new HTTP service using Frame, using the canonical `ctx, svc := frame.NewService(...)` bootstrap pattern.”
- “Create a new Frame plugin as a `WithXxx` option that registers a queue subscriber.”
- “Add a datastore setup using `WithDatastore` and a migration step.”
- “Generate OpenAPI specs with `frame openapi` and register `openapi.Option()`.”

## Frame Plugin Mental Model

A plugin is a `frame.Option` helper that configures a `Service` and registers startup/cleanup hooks. See `docs/plugins-options.md`.

## Don’ts

- Don’t assume `NewService` returns `*Service` directly; it returns `(context.Context, *Service)`.
- Don’t assume `WithName` accepts a handler; use `WithHTTPHandler` separately.

## Blueprint Extension Rules (AI-Safe)

When working with Frame Blueprints, always **extend** by default:

- Add new items without removing or replacing existing ones.
- Blueprint merges are additive; duplicates are ignored (base wins).

This preserves system stability while allowing incremental expansion.
