# Frame Blueprints (Extensible, Additive)

Frame Blueprints are a **compile-time contract** for describing services in a machine-friendly way. A blueprint is additive by default: **adding entries extends the system and never invalidates existing definitions**.

## Core Principles

- **Additive merge**: New entries are added; existing entries are preserved.
- **Stable identifiers**: Items are keyed by stable names (routes, queues, plugins).
- **Explicit overrides only**: Updates to existing items require explicit `override: true`.
- **No implicit deletes**: Removing items requires `remove: true`.

These rules keep blueprints safe for AI agents and humans: extension is the default behavior, not replacement.

## Blueprint Format (YAML)

```yaml
service: users
http:
  - name: list-users
    method: GET
    route: /users
    handler: GetUsers
plugins:
  - name: logging
  - name: telemetry
queues:
  - name: user-events
    publisher: user-events
    topic: mem://events
```

## Extension Example

### Base

```yaml
service: users
http:
  - name: list-users
    method: GET
    route: /users
    handler: GetUsers
```

### Extension (add new route)

```yaml
http:
  - name: create-user
    method: POST
    route: /users
    handler: CreateUser
```

### Result (merged)

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

## Override and Remove Semantics

If you need to modify or remove a definition, it must be explicit:

```yaml
http:
  - name: list-users
    method: GET
    route: /users
    handler: GetUsersV2
    override: true
  - name: create-user
    remove: true
```

## Merge Rules (Deterministic)

- Lists are merged by `name`.
- Maps are merged by key.
- Scalars are replaced **only** when `override: true` is set.
- `remove: true` deletes an item if it exists; otherwise it is ignored.

## Why This Matters

AI agents should be able to **extend** a service safely without breaking existing behavior. These rules make blueprint changes monotonic and safe by default.
