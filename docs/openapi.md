# OpenAPI in Frame

Frame serves OpenAPI specs that are embedded at **compile time** and exposed through a deterministic, discoverable endpoint. No runtime parsing or dynamic discovery is required.

## Production Goals

- Compile-time OpenAPI specs baked into the binary
- Deterministic runtime behavior (byte-for-byte serving)
- Clear regeneration path (single command + `go generate`)
- CI-friendly generation and validation

## Quick Start (One Command)

Generate specs and wire them automatically:

```bash
go run github.com/pitabwire/frame/cmd/frame@latest openapi \
  -proto-dir proto \
  -out pkg/openapi/specs \
  -embed-dir pkg/openapi \
  -package openapi
```

This will:

- Run `buf generate` with the Connect OpenAPI plugin
- Write OpenAPI JSON into `pkg/openapi/specs`
- Generate `pkg/openapi/embed.go` with an `Option()` helper
- Add a `go:generate` directive for easy regeneration

Then enable in your service:

```go
package main

import (
	"context"
	"log"

	"github.com/pitabwire/frame"
	"your/module/pkg/openapi"
)

func main() {
	ctx, svc := frame.NewService(
		frame.WithName("users-service"),
		openapi.Option(),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
```

## Served Endpoints

When at least one spec is registered:

- `GET /debug/frame/openapi` returns a JSON index of specs
- `GET /debug/frame/openapi/{name}` returns a specific spec

Example response:

```json
{
  "specs": [
    {"name": "users", "filename": "users.json"},
    {"name": "billing", "filename": "billing.json"}
  ]
}
```

## Compile-Time Guarantee

Specs are embedded with `//go:embed` and registered as static byte slices. The runtime only serves those bytes; it does not parse or regenerate OpenAPI.

## Regenerate With `go generate`

After the first run, you can regenerate from the embed package:

```bash
go generate ./pkg/openapi
```

## Custom Base Path

```go
frame.WithOpenAPIBasePath("/debug/frame/openapi")
```

## CI Integration

Recommended pipeline step (example):

```bash
go run github.com/pitabwire/frame/cmd/frame@latest openapi --proto-dir proto --out pkg/openapi/specs --embed-dir pkg/openapi --package openapi

go test ./...
```

For CI, ensure `buf` is installed and available in PATH.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Specs not served | `openapi.Option()` not passed | Register the option in `NewService` |
| No specs found | Wrong output dir or embed path | Ensure `pkg/openapi/specs/*.json` exists |
| `buf generate` fails | Buf not installed or invalid workspace | Install `buf` and ensure `proto/` has a valid `buf.yaml` or `buf.work.yaml` |
