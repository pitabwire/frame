# Setup plans: abstract bulk one-shot work

Deploy-time work (schema migrate, permission manifests, root/bot bootstrap,
verification) should run as a **bulk setup plan** in a Job — not on every
runtime cold start, and not as ad-hoc Frame-only side effects.

Frame ships an **abstract** package, [`setup`](../setup), plus thin Service
adapters. The contract does not depend on Cloud Run, Helm, or Frame internals.

---

## Package `setup` (abstract)

### `Step`

```go
type Step interface {
    Name() string
    Run(ctx context.Context) error
}
```

| Contract | Meaning |
|----------|---------|
| **Idempotent** | Re-run after success is no-op or upsert |
| **Fail-closed** | Non-nil error aborts the plan |
| **Named** | Stable unique name within a `Registry` |

Sugar: `setup.Func{StepName: "migrate", Fn: fn}`.

Well-known names (conventions only):

| Constant | Name | Typical use |
|----------|------|-------------|
| `setup.NameMigrate` | `migrate` | Schema migrations |
| `setup.NamePermissions` | `permissions` | Publish permission manifest |
| `setup.NameBootstrap` | `bootstrap` | Root/bot seed, tuples |
| `setup.NameVerify` | `verify` | Post-setup checks |

### `Registry` — bulk execution

```go
reg := setup.NewRegistry()
reg.RegisterFunc(setup.NameMigrate, migrateFn)
reg.Register(myBootstrapStep)
reg.RegisterFunc(setup.NameVerify, verifyFn)

// all registered, registration order
err := reg.RunAll(ctx)

// explicit subset / order
err := reg.Run(ctx, setup.NameMigrate, setup.NamePermissions, setup.NameBootstrap)
```

- Same name registered twice → **replace** function, **keep** position.
- Unknown name → `setup.ErrUnknownStep`.
- Empty registry / empty selection → `setup.ErrEmptyPlan`.

### `Selection` — process mode (pure)

```go
sel := setup.Select(os.Args[1:], doSetupFlag, setupTasksCSV)
// or setup.SelectFromOS(doSetup, csv)

if sel.Active {
    return reg.Run(ctx, sel.Names...) // empty Names => all registered
}
// else: long-running server
```

| Input | `Active` | `Names` |
|-------|----------|---------|
| argv `setup migrate permissions` | true | `[migrate, permissions]` |
| argv `setup` | true | empty → run all |
| `DO_SETUP=true` | true | CSV or all |
| `FRAME_SETUP_TASKS=a,b` | true | `[a,b]` |
| argv `migrate` alone | **false** | (legacy migrate path) |

---

## Frame adapters (thin)

| API | Role |
|-----|------|
| `svc.Setup() *setup.Registry` | Lazy registry on the Service |
| `frame.WithSetupStep(step)` | Register abstract `setup.Step` |
| `frame.WithSetupFunc(name, fn)` | Register name + `func(ctx) error` |
| `frame.WithSetupTask(name, fn)` | Register `func(ctx, *Service) error` when step needs Service |
| `svc.RunSetup(ctx, names...)` | `Setup().Run` + config selection |
| `frame.IsSetupMode(cfg)` / `SetupSelection(cfg)` | Config/argv selection |
| `frame.WithPermissionRegistration(sd)` | Registers `permissions` **Step** + optional runtime PreStart |

Config (`config.ConfigurationDefault`):

| Env / field | Purpose |
|-------------|---------|
| `DO_SETUP` | Force setup mode |
| `FRAME_SETUP_TASKS` | CSV step list when not using argv |
| `PERMISSIONS_REGISTER_ON_START` | Runtime PreStart for manifests (default `true`; set `false` when Job owns it) |
| `PERMISSIONS_REGISTRATION_URL` | Tenancy register URL (required for permissions step) |

---

## Application pattern

```go
package main

import (
    "context"

    "github.com/pitabwire/frame/v2"
    "github.com/pitabwire/frame/v2/config"
    "github.com/pitabwire/frame/v2/setup"
    "github.com/pitabwire/util"
)

func main() {
    ctx := context.Background()
    cfg, err := config.LoadWithOIDC[MyConfig](ctx)
    if err != nil {
        util.Log(ctx).WithError(err).Fatal("config")
    }

    ctx, svc := frame.NewServiceWithContext(ctx,
        frame.WithConfig(&cfg),
        frame.WithDatastore(),
    )

    // Abstract steps (no Service coupling when possible).
    svc.Setup().RegisterFunc(setup.NameMigrate, func(ctx context.Context) error {
        return repository.Migrate(ctx, svc.DatastoreManager(), cfg.GetDatabaseMigrationPath())
    })
    svc.Setup().RegisterFunc(setup.NameBootstrap, func(ctx context.Context) error {
        return business.EnsureRootAuthorization(ctx, deps)
    })
    svc.Setup().RegisterFunc(setup.NameVerify, func(ctx context.Context) error {
        return business.ConfirmReady(ctx, deps)
    })

    // Permissions step registered abstractly via Frame helper.
    sd := myv1.File_....Services().ByName("MyService")
    svc.Init(ctx, frame.WithPermissionRegistration(sd))

    // --- one-shot Job ---
    if frame.IsSetupMode(&cfg) {
        if err := svc.RunSetup(ctx); err != nil {
            util.Log(ctx).WithError(err).Fatal("setup plan failed")
        }
        return
    }

    // --- runtime ---
    if err := svc.Run(ctx, ""); err != nil {
        util.Log(ctx).WithError(err).Fatal("server stopped")
    }
}
```

**Standalone (no Service)** — same package, pure:

```go
reg := setup.NewRegistry()
reg.RegisterFunc(setup.NameMigrate, migrateFn)
reg.RegisterFunc(setup.NameVerify, verifyFn)

sel := setup.SelectFromOS(false, os.Getenv("FRAME_SETUP_TASKS"))
if !sel.Active {
    // serve...
    return
}
if err := reg.Run(ctx, sel.Names...); err != nil {
    log.Fatal(err)
}
```

---

## Cloud Run / Helm

### Job (setup plan)

```hcl
# After apps adopt the setup package:
args = ["setup", "migrate", "permissions", "bootstrap", "verify"]

env = {
  PERMISSIONS_REGISTRATION_URL = "https://tenancy.stawi.org/_internal/register/permissions"
  # OAuth/Keto as needed so permissions step can authenticate
}
```

### Runtime service

```hcl
env = {
  PERMISSIONS_REGISTER_ON_START = "false"  # Job owns manifests
  # do not require setup argv
}
```

### Legacy

| Still supported | Notes |
|-----------------|--------|
| argv `migrate` / `DO_MIGRATION` | `DoDatabaseMigrate()` only — **not** a full setup plan |
| PreStart permission publish | Default on until you set `PERMISSIONS_REGISTER_ON_START=false` |

---

## Why not “migrate only” or “register on every start”?

| Approach | Problem |
|----------|---------|
| Job runs only schema migrate | Apps often exit before any permission/bootstrap code |
| Runtime PreStart every scale-from-zero | Extra tenancy traffic; async fail-open hides errors |
| **Bulk setup plan** | Ordered, fail-closed, re-runnable, abstract steps |

---

## Adoption checklist

1. Depend on a Frame release that includes package `setup`.
2. Register steps on `svc.Setup()` (migrate, bootstrap, verify, …).
3. Keep `WithPermissionRegistration` for the permissions step (or register your own `setup.Step`).
4. Branch on `frame.IsSetupMode` / `setup.Selection.Active` before `svc.Run`.
5. Point the Cloud Run Job at `["setup", …]`.
6. Set `PERMISSIONS_REGISTER_ON_START=false` on runtime replicas.
7. Remove ad-hoc early-return migrate paths once the plan covers them.

---

## API map

```
setup.Step / setup.Func          abstract unit of work
setup.Registry                   bulk register + Run / RunAll
setup.Selection / Select         pure mode + name list
        │
        ▼
frame.Service.Setup()            holds Registry
frame.WithSetupStep/Func/Task    registration helpers
frame.RunSetup / IsSetupMode     Service + config wiring
frame.WithPermissionRegistration registers permissions Step (+ optional PreStart)
```
