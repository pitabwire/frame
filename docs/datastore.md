# Datastore (GORM + Connection Pools)

Frame's datastore layer provides pooled database connections and migration management on top of GORM.

## Overview

- `datastore.Manager` manages named pools.
- `datastore/pool` provides GORM-backed connections and tuning.
- `datastore/migration` supports migration patches.
- `tenancy/` package provides pluggable RLS enforcement via providers.

## Quick Start

```go
_, svc := frame.NewService(
    frame.WithDatastore(),
)

// default pool
if db := svc.DatastoreManager().DB(ctx, false); db != nil {
    _ = db.Exec("select 1").Error
}
```

## Configure via Environment

Set `DATABASE_URL` (and optional `REPLICA_DATABASE_URL`) in config. Frame auto-wires pools.

```bash
export DATABASE_URL=postgres://user:pass@host:5432/dbname?sslmode=disable
```

## Multiple Pools

```go
_, svc := frame.NewService(
    frame.WithDatastoreConnectionWithName("primary", dsn, false),
    frame.WithDatastoreConnectionWithName("replica", dsnReplica, true),
)

primary := svc.DatastoreManager().DBWithPool(ctx, "primary", false)
```

## Migrations

When `DO_MIGRATION=true`, Frame creates a migration pool and runs migrations.

```go
err := svc.DatastoreManager().Migrate(ctx, pool, "./migrations", &MyModel{})
```

## Tuning

Use config or pool options:

- Max open connections
- Max idle connections
- Max connection lifetime
- Prepared statements

## Tenancy

Tenancy enforcement lives in the top-level `tenancy/` package. The
default Postgres provider installs Row-Level Security policies on every
model that satisfies `tenancy.Tenanted` (which `data.BaseModel` does
out of the box), and binds per-request tenancy state to each database
connection through pgxpool acquire/release hooks. Application code
never references `tenant_id` or `partition_id` directly.

### Wiring

```go
_, svc := frame.NewService(
    frame.WithDatastore(), // installs default Postgres adapter + RLS provider
)

// Register the lightweight claims interceptor on Connect handlers
// AFTER your authentication interceptor:
options := connect.WithInterceptors(
    authInterceptor,
    tenancy.NewClaimsInterceptor(),
)
```

### Building / extending tenancy claims

```go
// Claims are derived from security.AuthenticationClaims by default.
got := tenancy.ClaimsFromContext(ctx)

// For service-on-behalf-of flows, extend with additional partitions:
ctx = tenancy.WithExtraPartitions(ctx, "branch-2", "branch-3")

// For job workers reconstructing claims from queue metadata, build
// Claims explicitly and bind them:
ctx = tenancy.WithClaims(ctx, &tenancy.Claims{
    TenantID:     "T1",
    PartitionIDs: []string{"P1"},
    AccessID:     "A1",
})

// For admin scripts or migrations that legitimately need full-table
// access, bypass enforcement explicitly:
ctx = tenancy.WithSkipEnforcement(ctx)
```

### Performance: prefer the interceptor over auth-claim fallback

`tenancy.ClaimsFromContext` has a three-tier fallback:
1. Explicit `*tenancy.Claims` bound via `tenancy.WithClaims` (fastest — no allocation).
2. Derived from `security.AuthenticationClaims` if present (allocates a fresh `*Claims` every call).
3. `nil` if neither is bound.

The Postgres tenancy provider's connection-acquire hook calls
`ClaimsFromContext` on every connection acquired from the pool. For
high-throughput services, register `tenancy.NewClaimsInterceptor()`
after your authentication interceptor so the derived claims are bound
once per request:

```go
options := connect.WithInterceptors(
    authInterceptor,
    tenancy.NewClaimsInterceptor(),  // pre-binds Claims so the hot path is path 1
)
```

Without the interceptor, requests with auth claims still work
correctly — they just pay an extra allocation per connection acquire.

### Job workers: write-path tenancy

`data.BaseModel.BeforeCreate` reads from `security.AuthenticationClaims`
(via `security.ClaimsFromContext`) to populate `TenantID`,
`PartitionID`, and `AccessID` automatically. Job workers that build
`tenancy.Claims` from queue metadata without also pushing
`security.AuthenticationClaims` into the context must populate those
fields manually before calling `repo.Create`:

```go
entity := &MyModel{
    BaseModel: data.BaseModel{TenantID: msg.TenantID, PartitionID: msg.PartitionID},
    // ... other fields
}
err := repo.Create(ctx, entity)
```

Alternatively, push an `AuthenticationClaims` into the context so the
`BaseModel` hook fires automatically:

```go
auth := &security.AuthenticationClaims{
    TenantID: msg.TenantID, PartitionID: msg.PartitionID,
}
ctx := auth.ClaimsToContext(ctx)
// repo.Create(ctx, entity) now picks up TenantID/PartitionID from auth.
```

### One-shot calls are the encouraged path

Repositories continue to call `pool.DB(ctx, _)` — tenancy is applied
transparently. For multi-statement atomicity, use raw GORM:

```go
db := dbPool.DB(ctx, false)
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&e1).Error; err != nil { return err }
    if err := tx.Create(&e2).Error; err != nil { return err }
    return nil
})
```

The `*gorm.DB` is local to the closure — transactions are never
threaded through `context.Context`.

### Opting a model out

Embed `tenancy.UnscopedMarker` on tables that should not have RLS
installed (lookup tables, migration metadata):

```go
type LookupTable struct {
    ID string `gorm:"primaryKey"`
    tenancy.UnscopedMarker
}
```

### Custom provider

Swap the default provider via `frame.WithTenancyProvider`. Implementing
a new tenancy scheme is a matter of writing a `tenancy.Provider` plus,
if a new database is involved, a `dialect.DialectAdapter`.

### IMPORTANT: Postgres superuser bypasses RLS

Postgres SUPERUSER and roles with the `BYPASSRLS` attribute bypass
Row-Level Security policies entirely, **even with `FORCE ROW LEVEL
SECURITY`**. This is a Postgres design choice and applies regardless
of frame's wiring.

In production, services MUST connect to Postgres as a non-superuser
role without `BYPASSRLS`. If you connect as a superuser (which is the
default in many local-dev images), RLS will be silently disabled and
every query will return rows from every tenant.

Recommended production setup:
1. Create a dedicated application role (e.g., `app_user`) that is NOT
   a superuser and does NOT have `BYPASSRLS`.
2. Grant that role the privileges it needs on the application schema.
3. Connect from frame using that role's credentials.
4. Use a separate, privileged role only for migrations and operator
   tasks that must bypass RLS.

Frame's testcontainer-based integration tests work around this by
creating a non-superuser role inside the test setup; see
`tenancy/postgres/provider_test.go` for the pattern.

## API Reference (Key)

- `manager.NewManager(ctx)`
- `Manager.AddPool(ctx, name, pool)`
- `Manager.DB(ctx, readOnly)`
- `Manager.DBWithPool(ctx, name, readOnly)`
- `Manager.Migrate(ctx, pool, dir, models...)`
- `Manager.SaveMigration(ctx, pool, patches...)`

