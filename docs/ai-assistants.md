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
/internal/handlers/...
/internal/plugins/...
/internal/clients/...
/configs/...
```

## How to Ask AI for Frame Code

Use these prompt patterns:

- “Generate a new HTTP service using Frame, using the canonical `ctx, svc := frame.NewService(...)` bootstrap pattern.”
- “Create a new Frame plugin as a `WithXxx` option that registers a queue subscriber.”
- “Add a datastore setup using `WithDatastore` and a migration step.”

## Frame Plugin Mental Model

A plugin is a `frame.Option` helper that configures a `Service` and registers startup/cleanup hooks. See `docs/plugins-options.md`.

## Don’ts

- Don’t assume `NewService` returns `*Service` directly; it returns `(context.Context, *Service)`.
- Don’t assume `WithName` accepts a handler; use `WithHTTPHandler` separately.
