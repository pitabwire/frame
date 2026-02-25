# Datastore (GORM + Connection Pools)

Frame's datastore layer provides pooled database connections and migration management on top of GORM.

## Overview

- `datastore.Manager` manages named pools.
- `datastore/pool` provides GORM-backed connections and tuning.
- `datastore/migration` supports migration patches.
- `datastore/scopes` includes tenancy helpers.

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

`datastore/scopes/tenancy` provides helpers to apply tenant/partition constraints. Combined with `data.BaseModel` you can automatically scope models using claims from context.

## API Reference (Key)

- `manager.NewManager(ctx)`
- `Manager.AddPool(ctx, name, pool)`
- `Manager.DB(ctx, readOnly)`
- `Manager.DBWithPool(ctx, name, readOnly)`
- `Manager.Migrate(ctx, pool, dir, models...)`
- `Manager.SaveMigration(ctx, pool, patches...)`

