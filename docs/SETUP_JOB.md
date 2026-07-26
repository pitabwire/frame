# Setup job (migrate, permissions, bootstrap)

Cloud Run / Helm one-shot work should **not** live on every runtime start.
Frame provides ordered **setup tasks** for a dedicated Job.

## Why not only `migrate` / runtime PreStart?

| Approach | Problem |
|----------|---------|
| `DO_MIGRATION` / argv `migrate` alone | Many `cmd/main.go` paths migrate and **exit** before `Init` / PreStart, so permissions never run |
| Runtime PreStart every scale-from-zero | Extra tenancy POSTs on cold start; fails open (async) so you may not notice |
| **Setup job** | Fail-closed, ordered, idempotent steps before traffic |

## Config / argv

| Input | Meaning |
|-------|---------|
| argv `setup migrate permissions bootstrap` | Setup mode; run those tasks in order |
| argv `setup` | Setup mode; run **all** registered tasks |
| `DO_SETUP=true` | Setup mode |
| `FRAME_SETUP_TASKS=migrate,permissions` | Setup mode + task list (if not using argv) |
| `PERMISSIONS_REGISTER_ON_START=false` | Disable async PreStart registration on **runtime** replicas |
| `PERMISSIONS_REGISTRATION_URL` | Tenancy register endpoint (required for permissions task) |

Legacy argv `migrate` alone remains for backwards compatibility (`DoDatabaseMigrate()`).

## Service API

```go
ctx, svc := frame.NewServiceWithContext(ctx, frame.WithConfig(&cfg), frame.WithDatastore())

// App-owned steps
svc.AddSetupTask(frame.SetupTaskMigrate, func(ctx context.Context, s *frame.Service) error {
    return repository.Migrate(ctx, s.DatastoreManager(), cfg.GetDatabaseMigrationPath())
})
svc.AddSetupTask("bootstrap", func(ctx context.Context, s *frame.Service) error {
    return business.EnsureRootAuthorization(ctx, deps)
})

// Built-in: WithPermissionRegistration also registers SetupTaskPermissions
// and optionally PreStart (PERMISSIONS_REGISTER_ON_START).
svc.Init(ctx,
    frame.WithPermissionRegistration(sd),
    // other serve-time options omitted for setup-only jobs if desired
)

if frame.IsSetupMode(&cfg) {
    if err := svc.RunSetup(ctx); err != nil {
        util.Log(ctx).WithError(err).Fatal("setup failed")
    }
    return
}

// runtime serve path
_ = svc.Run(ctx, "")
```

Or via option:

```go
frame.WithSetupTask("bootstrap", fn)
```

## Cloud Run Job example

```hcl
# modules/frame-cloudrun-app migrate job (future default once apps adopt):
args = ["setup", "migrate", "permissions"]

# Runtime service env:
# PERMISSIONS_REGISTER_ON_START = "false"
# PERMISSIONS_REGISTRATION_URL  = unset or ignored for PreStart when false
```

## Task contracts

- **Idempotent**: re-run after success must be safe.
- **Fail-closed** in `RunSetup`: any task error fails the Job.
- **permissions**: uses the same HTTP upsert as PreStart, with retries, but returns error after exhaustion.

## Adoption checklist

1. Bump Frame to a release with setup API.
2. In each `cmd/main.go`, register migrate/bootstrap as setup tasks; call `RunSetup` when `IsSetupMode`.
3. Job args: `["setup", "migrate", "permissions", …]`.
4. Runtime: `PERMISSIONS_REGISTER_ON_START=false`.
5. Keep `PERMISSIONS_REGISTRATION_URL` on the **setup** Job env.
