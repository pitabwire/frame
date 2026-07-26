# Setup plans: abstract bulk one-shot work

Deploy-time work (schema migrate, permission manifests, root/bot bootstrap,
verification) runs as a **bulk setup plan** in a Job — not on every runtime
cold start.

Frame ships package [`setup`](../setup) plus thin Service adapters. The
contract does not depend on Cloud Run, Helm, or Frame HTTP serving.

**Permission manifests are never published at runtime PreStart.** Use the
`permissions` setup step so startup stays fast.

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

err := reg.RunAll(ctx) // registration order
err := reg.Run(ctx, setup.NameMigrate, setup.NamePermissions, setup.NameBootstrap)
```

### `Selection` — process mode (pure)

```go
sel := setup.Select(os.Args[1:], doSetupFlag, setupTasksCSV)
if sel.Active {
    return reg.Run(ctx, sel.Names...) // empty Names => all registered
}
```

| Input | Active | Names |
|-------|--------|-------|
| argv `setup migrate permissions` | true | `[migrate, permissions]` |
| argv `setup` | true | empty → all registered |
| `DO_SETUP=true` / `FRAME_SETUP_TASKS` | true | CSV or all |
| argv `migrate` alone | **false** | use `ShouldRunSetup` + `RunSetupForProcess` |

---

## Frame adapters

| API | Role |
|-----|------|
| `svc.Setup() *setup.Registry` | Lazy registry |
| `WithSetupStep` / `WithSetupFunc` | Register abstract steps |
| `WithSetupTask` | Step that needs `*Service` |
| `WithPermissionRegistration(sd)` | Registers **only** the `permissions` setup Step (no PreStart) |
| `ShouldRunSetup(cfg)` | Setup mode **or** legacy migrate |
| `RunSetupForProcess(ctx, cfg)` | Runs plan for this process then caller exits |
| `IsSetupMode` / `SetupSelection` | Config/argv selection |

### Config

| Env | Purpose |
|-----|---------|
| `DO_SETUP` | Force setup mode |
| `FRAME_SETUP_TASKS` | CSV step list |
| `PERMISSIONS_REGISTRATION_URL` | Required on the **setup Job** for permissions |
| `PERMISSIONS_REGISTER_ON_START` | **Deprecated / ignored** |

---

## Application pattern

```go
ctx, svc := frame.NewServiceWithContext(ctx, frame.WithConfig(&cfg), frame.WithDatastore())

svc.Setup().RegisterFunc(setup.NameMigrate, func(ctx context.Context) error {
    return repository.Migrate(ctx, svc.DatastoreManager(), cfg.GetDatabaseMigrationPath())
})
svc.Setup().RegisterFunc(setup.NameBootstrap, func(ctx context.Context) error {
    return business.EnsureRootAuthorization(ctx, deps)
})

sd := myv1.File_....Services().ByName("MyService")

if frame.ShouldRunSetup(&cfg) {
    // Light Init: enough for OAuth HTTP client + registered steps.
    svc.Init(ctx, frame.WithPermissionRegistration(sd))
    if err := svc.RunSetupForProcess(ctx, &cfg); err != nil {
        util.Log(ctx).WithError(err).Fatal("setup plan failed")
    }
    return
}

// Runtime — no permission POST on startup.
svc.Init(ctx, frame.WithHTTPHandler(...), frame.WithPermissionRegistration(sd), ...)
// WithPermissionRegistration still registers the step (harmless if never RunSetup);
// omit it on pure runtime if you prefer, as long as the setup Job image includes it.
_ = svc.Run(ctx, "")
```

**Legacy `migrate` argv:** `ShouldRunSetup` is true; `RunSetupForProcess` runs
registered well-known steps in order: migrate → bootstrap → permissions → verify.

---

## Cloud Run

### Job

```hcl
args = ["setup", "migrate", "permissions", "bootstrap"]
# or keep ["migrate"] — RunSetupForProcess still runs well-known registered steps

env = {
  PERMISSIONS_REGISTRATION_URL = "https://tenancy.stawi.org/_internal/register/permissions"
  # OAuth/Keto as needed so the permissions step can authenticate
}
```

### Runtime service

```hcl
# Fast cold start: no setup argv, no permission registration on PreStart.
# PERMISSIONS_REGISTRATION_URL optional on runtime.
```

---

## Adoption checklist

1. Frame ≥ **v2.0.17** (no runtime PreStart permissions).
2. Register `migrate` / `bootstrap` / `verify` on `svc.Setup()`.
3. `WithPermissionRegistration(sd)` so the Job can run `permissions`.
4. Branch: `if frame.ShouldRunSetup(&cfg) { RunSetupForProcess; return }`.
5. Job args: `["setup", "migrate", "permissions", …]` (or legacy `migrate`).
6. Runtime must not rely on startup permission POSTs.

---

## API map

```
setup.Step / Func          abstract unit of work
setup.Registry             bulk register + Run / RunAll
setup.Selection / Select   pure mode + name list
        │
        ▼
frame.Service.Setup()              holds Registry
frame.WithSetupStep/Func/Task      registration helpers
frame.ShouldRunSetup               setup job OR legacy migrate
frame.RunSetupForProcess           execute plan for this process
frame.WithPermissionRegistration   permissions Step only (no PreStart)
```
