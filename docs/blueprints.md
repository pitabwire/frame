# Frame Blueprints

Frame Blueprints are machine-friendly YAML/JSON definitions that generate complete services. They are **additive by default**: new entries extend the system without invalidating existing definitions.

## Goals

- Deterministic generation
- Canonical bootstrap pattern
- Minimal inputs with strong defaults
- Safe extension for AI and humans

## Blueprint Format (v0.1)

```yaml
schema_version: 0.1
runtime_mode: polylith
service:
  name: users
  module: github.com/acme/users
  service_id: users
  service_group: profile
  port: ":8080"
http:
  - route: /users
    method: GET
    handler: GetUsers
plugins:
  - telemetry
  - logger
datastore:
  migrate: true
  primary_url_env: DATABASE_URL
queues:
  - publisher: user-events
    url: mem://events
  - subscriber: user-events
    url: mem://events
    handler: HandleUserEvent
```

## Runtime Mode in Blueprints

- `runtime_mode`: `monolith` or `polylith`
- `service.service_id`: required for polylith
- `service.service_group`: optional

Monolith example (single mux + single runtime service):

```yaml
schema_version: 0.1
runtime_mode: monolith
service:
  name: profile-monolith
  port: ":8080"
http:
  - route: /devices
    method: GET
    handler: GetDevices
  - route: /geo
    method: GET
    handler: GetGeo
```

Monolith generation contract:

- Exactly one `http.ServeMux`
- Exactly one `frame.NewService(...)`
- Exactly one `svc.Run(ctx, port)`
- Multiple routes are composed into the single mux

## Additive Extension Rules

- **Additive merge**: New entries are added; existing entries are preserved.
- **Base wins on duplicates**: If a route/queue/plugin key already exists, the original entry remains.
- **No deletes**: Blueprint merges never remove existing entries.

These rules keep blueprints safe for AI agents and humans: extension is the default behavior, not replacement.

### Extension Example

Base:

```yaml
service: users
http:
  - name: list-users
    method: GET
    route: /users
    handler: GetUsers
```

Extension:

```yaml
http:
  - name: create-user
    method: POST
    route: /users
    handler: CreateUser
```

Merged result:

```yaml
http:
  - name: list-users
    method: GET
    route: /users
    handler: GetUsers
  - name: create-user
    method: POST
    route: /users
    handler: CreateUser
```

If a route uses the same `name` (or the same `method` + `route`), the base entry remains and the overlay is ignored.

- Same blueprint inputs produce identical output.
- Canonical bootstrap pattern is enforced.
- Options are grouped in one block.

## Output Directory

By default, `frame build` writes into `./_generated`. For AI workflows, use `--out` to isolate generated code:

```bash
go run ./cmd/frame build blueprints/polylith.yaml --out ./_generated
```

Build monolith blueprint:

```bash
go run ./cmd/frame build blueprints/monolith.yaml --out ./_generated
```

Generated files (monolith):

- `cmd/main.go`
- `pkg/services/<name>/routes.go`
- `pkg/queues/<name>/queues.go` (when queues are defined)


## Blueprint Schema

A JSON schema is provided at `blueprints/blueprint.schema.json` for tooling and AI validation.
