# package setup

Abstract **bulk setup plans**: ordered, fail-closed, idempotent steps for
deploy Jobs (migrate, permissions, bootstrap, verify, …).

No dependency on Frame `Service`, Cloud Run, or HTTP — only `context` and
logging via `util.Log`.

| Type | Role |
|------|------|
| `Step` | `Name()` + `Run(ctx)` |
| `Func` | Function-backed step |
| `Registry` | Register + `Run` / `RunAll` |
| `Selection` / `Select` | Pure argv/env → which steps to run |

Full documentation: [docs/SETUP_JOB.md](../docs/SETUP_JOB.md).

```go
reg := setup.NewRegistry()
reg.RegisterFunc(setup.NameMigrate, migrate)
reg.RegisterFunc(setup.NameVerify, verify)

sel := setup.Select(os.Args[1:], false, os.Getenv("FRAME_SETUP_TASKS"))
if sel.Active {
    return reg.Run(ctx, sel.Names...)
}
```
