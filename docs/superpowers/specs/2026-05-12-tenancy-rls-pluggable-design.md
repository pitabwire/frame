# Tenancy & RLS — Pluggable, Database-Agnostic Design

**Status:** Approved (design)
**Date:** 2026-05-12
**Owner:** Peter Bwire
**Related code:** `datastore/pool/rls.go`, `datastore/scopes/tenancy.go`, `security/interceptors/connect/tenancy_tx.go`, `data/model.go`, `security/security_claims.go`

---

## Motivation

The current tenancy enforcement is correct but tightly coupled:

- Row-Level Security is hard-coded to Postgres (`gorm.io/driver/postgres`, `pgxpool`, plpgsql function) inside `datastore/pool/rls.go`.
- `datastore/scopes/tenancy.go` duplicates the RLS guarantee at the GORM-builder level, creating two sources of truth.
- `Pool.WithTenancy` and `Pool.WithRequestTx` couple tenancy to ambient transactions threaded through `context.Context` via `ContextWithTx` / `TxFromContext`. A single RPC owns one large transaction across handler → business → repository → repository's helpers. Cross-system reasoning about atomicity becomes implicit.
- The Connect tenancy interceptor opens a request-scoped transaction even for streaming RPCs, which holds a database connection for the lifetime of the stream.
- Adding any other database (or even a second tenancy mechanism, e.g. schema-per-tenant) requires touching the pool, scopes, interceptor, and migration paths simultaneously.

This spec replaces the coupled implementation with a small, pluggable architecture that:

1. Isolates database-specific driver concerns behind `dialect.DialectAdapter`.
2. Isolates tenancy enforcement behind `tenancy.Provider`.
3. Removes transaction-in-context plumbing — `pool.DB(ctx, _)` returns a tenancy-aware `*gorm.DB`; multi-statement atomicity is opt-in via raw GORM.
4. Introduces a dedicated, immutable `tenancy.Claims` value object derived by default from `security.AuthenticationClaims`, extended additively when callers need cross-partition access.
5. Deletes the redundant `datastore/scopes` package entirely.

---

## Non-goals

- Supporting a second database in this change. Postgres is the only concrete adapter/provider shipped; the abstractions exist so a second database can be added later by writing one adapter + one provider.
- Reworking `data.BaseModel`'s `BeforeCreate` / `BeforeUpdate` hooks. Those continue to default `tenant_id`/`partition_id` from auth claims at insert time — that is application-side defaulting, not enforcement.
- Reworking `BaseRepository` to be tx-aware. Repositories continue to call `pool.DB(ctx, readOnly)`. Multi-statement atomicity is the caller's responsibility (GORM `db.Transaction(fn)`).
- Reworking authorization (`security/authorizer`). The existing tenancy-access interceptor (`tenancy_access.go`) is unchanged.

---

## Architecture

### Package layout

```
frame/
├── tenancy/                              ← NEW (top-level cross-cutting package)
│   ├── claims.go                         ← Claims, ClaimsFromAuth, WithClaims, ClaimsFromContext, WithExtraPartitions
│   ├── provider.go                       ← Provider interface, Capabilities, ModelInfo
│   ├── marker.go                         ← Tenanted, Unscoped marker interfaces
│   ├── enrollment.go                     ← reflection-based model detection (moved from rls.go)
│   ├── interceptor.go                    ← NewClaimsInterceptor (Connect)
│   ├── claims_test.go                    ← unit tests (pure data)
│   ├── interceptor_test.go               ← testcontainer Postgres
│   └── postgres/
│       ├── provider.go                   ← Postgres-RLS Provider (Install + Wire)
│       ├── sql.go                        ← SQL fragments (function, policy)
│       └── provider_test.go              ← testcontainer Postgres
│
├── datastore/
│   ├── dialect/                          ← NEW (driver abstraction)
│   │   ├── adapter.go                    ← DialectAdapter, AcquireHook, ReleaseHook, DialectConn, ConnectionOptions
│   │   └── postgres/
│   │       ├── postgres.go               ← pgxpool + gorm postgres + hook chain
│   │       ├── dsn.go                    ← DSN normalisation (moved from pool/connection.go)
│   │       ├── dsn_test.go               ← unit tests
│   │       └── postgres_test.go          ← testcontainer Postgres (advisory lock, etc.)
│   ├── pool/
│   │   ├── interface.go                  ← Pool: DB, AddConnection, Close, CanMigrate, SaveMigration, Migrate (NO tenancy methods)
│   │   ├── implementation.go             ← uses DialectAdapter + tenancy.Provider
│   │   ├── connection.go                 ← delegates to adapter
│   │   ├── options.go                    ← + WithDialectAdapter, WithTenancyProvider
│   │   └── implementation_test.go        ← testcontainer Postgres (extended)
│   ├── scopes/                           ← DELETED
│   ├── interface.go                      ← unchanged
│   ├── repository.go                     ← unchanged
│   └── manager/manager.go                ← unchanged
│
├── data/model.go                         ← + SetTenantID, SetPartitionID, SetAccessID; implements tenancy.Tenanted
├── security/
│   └── interceptors/connect/
│       ├── tenancy_tx.go                 ← DELETED
│       └── tenancy_access.go             ← unchanged
└── options_datastore.go                  ← + WithTenancyProvider; default to Postgres provider; + svc.TenancyProvider() accessor
```

### Dependency directions

```
options_datastore.go (frame package)
        │
        ▼
datastore/pool ───▶ datastore/dialect ───▶ (postgres adapter)
        │                  ▲
        ▼                  │
   tenancy ────────────────┘ (provider registers hooks on adapter)
        │
        ▼
   security (read-only — for auth claims)
```

- `tenancy` depends only on `security` and `gorm.io/gorm`. No `datastore` import.
- `dialect` depends only on `database/sql` and `gorm.io/gorm`. No `tenancy` import.
- `pool` depends on `dialect` and `tenancy`, plus `migration`.
- `data` depends on `gorm` and `security`. Implements `tenancy.Tenanted` structurally; `tenancy` never imports `data`.

---

## `tenancy.Claims` — the dedicated object

```go
// Claims is the storage-layer view of a principal's tenancy. Immutable.
type Claims struct {
    TenantID     string
    PartitionIDs []string
    AccessID     string
    Skip         bool
}

func (c *Claims) IsEmpty() bool

// ExtendPartitions returns a new Claims with additional partition IDs
// merged in. Preserves TenantID, AccessID, and Skip unchanged. Deduplicates
// partitions. Existing order is preserved; new IDs appended after.
func (c *Claims) ExtendPartitions(partitionIDs ...string) *Claims

// ClaimsFromAuth derives Claims from auth claims using the frame default
// mapping. Not overridable — explicit construction is the override path.
func ClaimsFromAuth(ctx context.Context, auth *security.AuthenticationClaims) *Claims

// WithClaims binds Claims to ctx.
func WithClaims(ctx context.Context, c *Claims) context.Context

// WithExtraPartitions reads the current Claims from ctx, extends them
// with the supplied partition IDs, and binds the extended Claims to a
// child ctx. Returns ctx unchanged if there are no current claims.
func WithExtraPartitions(ctx context.Context, partitionIDs ...string) context.Context

// ClaimsFromContext returns the bound Claims with graceful fallback:
//   1. Explicit Claims bound via WithClaims
//   2. Derived from security.AuthenticationClaims if present
//   3. nil (system services, migrations)
func ClaimsFromContext(ctx context.Context) *Claims
```

### Default `ClaimsFromAuth` mapping

| `tenancy.Claims` field | Source |
|---|---|
| `TenantID`     | `auth.GetTenantID()` |
| `PartitionIDs` | `auth.GetPartitionIDs()` |
| `AccessID`     | `auth.GetAccessID()` |
| `Skip`         | `auth.IsInternalSystem() OR security.IsTenancyChecksOnClaimSkipped(ctx)` |

### Why immutable + additive

- Concurrent goroutines observing the same parent ctx see consistent state.
- The TenantID never silently changes — the only way to act under a different tenant is to construct a new `Claims` directly and bind it (auditable).
- Cross-partition extension (operator-spanning-branches, analyst aggregating across groups) is expressed by additive merge, never by overriding.

---

## `tenancy.Provider` — the abstraction

```go
type Provider interface {
    Name() string
    Capabilities() Capabilities

    // Install applies storage-side enforcement schema (RLS, views, etc.)
    // for the supplied models. Called once during pool.Migrate. Must be
    // idempotent — Frame re-runs migration on every boot.
    Install(ctx context.Context, db *gorm.DB, models []ModelInfo) error

    // Wire registers the provider's per-request enforcement with the
    // dialect and/or GORM. Called once during pool initialisation. The
    // provider decides whether to use connection-acquire hooks, a GORM
    // plugin, or both.
    Wire(adapter dialect.DialectAdapter, db *gorm.DB) error
}

type Capabilities struct {
    // EnforcesAtStorage is true when the provider installs DB-side rules
    // that block access without per-query gating (RLS, views).
    EnforcesAtStorage bool
}

// ModelInfo describes one tenancy-enrolled model for Install. Built by
// the tenancy package via reflection over BaseModel-embedding migration
// models. Providers do not reimplement enrollment detection.
type ModelInfo struct {
    Table           string
    TenantColumn    string // default "tenant_id"
    PartitionColumn string // default "partition_id"
}
```

### Enrollment

`tenancy.enrollment.go` exposes:

```go
// EnrolledModels filters migration models that satisfy the Tenanted
// interface and returns ModelInfo for each. Models implementing
// tenancy.Unscoped opt out.
func EnrolledModels(db *gorm.DB, models []any) ([]ModelInfo, error)

// Tenanted is the structural interface a model must satisfy to be
// enrolled in tenancy enforcement. data.BaseModel satisfies it; any
// custom model that wants enrollment satisfies it explicitly. The
// tenancy package never imports data.
type Tenanted interface {
    GetTenantID() string
    GetPartitionID() string
    GetAccessID() string
    SetTenantID(string)
    SetPartitionID(string)
    SetAccessID(string)
}

// Unscoped is the marker interface for models that should NOT have
// tenancy enforcement installed (lookup tables, migration metadata).
type Unscoped interface {
    tenancyUnscoped() // unexported method discourages accidental implementation
}
```

`data.BaseModel` will gain the three `Set*` methods to satisfy `Tenanted`. Detection is structural rather than reflective — equivalent in practice to "embeds `data.BaseModel`" because BaseModel is the canonical implementor — but decoupled from a specific type, so the `tenancy` package never imports `data` and downstream services can roll their own tenanted base type if they need to.

### Postgres provider concrete behaviour

`tenancy/postgres/provider.go`:

- **`Install`**:
  1. `CREATE OR REPLACE FUNCTION app_tenancy_matches(...)` — same plpgsql function as today.
  2. For each `ModelInfo`:
     - `ALTER TABLE <table> ENABLE ROW LEVEL SECURITY`
     - `ALTER TABLE <table> FORCE ROW LEVEL SECURITY`
     - `DROP POLICY IF EXISTS app_tenancy_isolation ON <table>`
     - `CREATE POLICY app_tenancy_isolation ON <table> FOR ALL USING (app_tenancy_matches(<tenant_col>, <partition_col>)) WITH CHECK (app_tenancy_matches(<tenant_col>, <partition_col>))`
  3. Identifier quoting via `adapter.QuoteIdentifier`.

- **`Wire`**: registers two hooks on the adapter.
  - `BeforeAcquire(ctx, conn)`: reads `tenancy.ClaimsFromContext(ctx)`. If non-nil and `!Skip`, emits `SELECT set_config('app.tenant_id', $1, false), set_config('app.partition_id', $2, false)` on `conn` (`is_local=false` → session-scoped, no transaction required). PartitionIDs serialised as comma-separated string (same format the RLS function expects today).
  - `AfterRelease(ctx, conn)`: emits `RESET app.tenant_id; RESET app.partition_id` on `conn`. Combined with `MaxIdleConns=0` on `sql.DB`, this guarantees no leaked session state between requests.

- **`Capabilities`**: `{EnforcesAtStorage: true}`.

### Failure isolation

- If `BeforeAcquire` returns an error, the adapter logs at WARN and rejects the acquire (pgxpool drops the conn). The next query attempt acquires a fresh conn; if the underlying claims issue persists, the error bubbles up cleanly. No partially-scoped queries possible.
- If `Install` fails during `pool.Migrate`, the migration fails; Kubernetes retries the job. Same semantics as today.

---

## `dialect.DialectAdapter` — driver abstraction

```go
type DialectAdapter interface {
    Name() string

    NormalizeDSN(raw string) (string, error)
    OpenConnection(ctx context.Context, dsn string, opts ConnectionOptions) (gorm.Dialector, *sql.DB, error)

    AdvisoryLock(ctx context.Context, db *gorm.DB, id int64) (release func(), err error)
    IsRelationAlreadyExistsErr(err error) bool
    QuoteIdentifier(name string) string

    RegisterAcquireHook(fn AcquireHook) error
    RegisterReleaseHook(fn ReleaseHook) error
}

type AcquireHook func(ctx context.Context, conn DialectConn) error
type ReleaseHook func(ctx context.Context, conn DialectConn) error

// DialectConn is the minimal surface a hook needs. Each adapter wraps
// its native conn (e.g. *pgx.Conn) behind this interface so no driver
// types leak past the boundary.
type DialectConn interface {
    Exec(ctx context.Context, query string, args ...any) error
}

type ConnectionOptions struct {
    MaxOpen                int
    MaxLifetime            time.Duration
    PreferSimpleProtocol   bool
    SkipDefaultTransaction bool
    InsertBatchSize        int
    PreparedStatements     bool
    Logger                 gormlogger.Interface
}
```

### Postgres adapter (`dialect/postgres/postgres.go`)

- Takes over the body of today's `pool/connection.go`:
  - `cleanPostgresDSN` → `NormalizeDSN`.
  - `pgxpool.ParseConfig` + `pgxpool.NewWithConfig` + `otelpgx.NewTracer` + `otelpgx.RecordStats` + `stdlib.GetPoolConnector` + `sql.OpenDB` + `gorm.Open(postgres.New(...), ...)` → `OpenConnection`.
- Takes over `pool/implementation.go`:
  - `acquireMigrationLock` → `AdvisoryLock` (preserves the 82548391244719 advisory lock ID and retry semantics).
  - `isRelationAlreadyExistsErr` → `IsRelationAlreadyExistsErr`.
- Takes over `pool/rls.go`:
  - `pgQuoteIdent` → `QuoteIdentifier`.
- **Hook plumbing**:
  - Maintains internal slices of registered acquire/release hooks.
  - Wires `pgxpool.Config.BeforeAcquire` and `AfterRelease` to walk the hook chain:
    - `BeforeAcquire`: for each registered acquire hook, wrap the `*pgx.Conn` in a `DialectConn` adapter and call the hook; return `false` (drop conn) on any error; log at WARN.
    - `AfterRelease`: for each registered release hook, run; return `false` (destroy conn) on any error so a poisoned conn isn't reused; log at WARN.
- **Pool tuning** moves with `OpenConnection`: `MaxOpen` clamping at `MaxInt32`, `MaxLifetime`, `MaxIdleConns=0` on the `sql.DB`, etc.

---

## `datastore/pool` changes

### Removed

- `WithTenancy`, `WithRequestTx` methods from `Pool` interface.
- `ContextWithTx`, `TxFromContext` functions.
- Import of `datastore/scopes`. Call to `Scopes(scopes.TenancyPartition(ctx))` in `DB`.
- All Postgres driver imports (`gorm.io/driver/postgres`, `pgxpool`, `otelpgx`, `pgconn`, `stdlib`) — moved to `dialect/postgres`.
- `rls.go` and its functions (`enableRowLevelSecurity`, `applyTenancyPolicy`, `embedsBaseModel`, `tableNameFor`, `pgQuoteIdent`, `appTenancyMatchesFn`) — moved to `tenancy/postgres/provider.go` and `tenancy/enrollment.go`.

### Added options

```go
// WithDialectAdapter sets the dialect adapter for this pool.
// Default: dialect/postgres adapter.
func WithDialectAdapter(adapter dialect.DialectAdapter) Option

// WithTenancyProvider sets the tenancy provider for this pool.
// Default: tenancy/postgres provider (Postgres-RLS).
// nil disables tenancy enforcement (used in tests that want raw access).
func WithTenancyProvider(prov tenancy.Provider) Option
```

### `pool.NewPool(ctx, opts...) Pool`

Signature change: accept options at construction so adapter/provider can be configured. The existing `pool.NewPool(_ context.Context) Pool` form remains valid (uses defaults).

### Pool initialisation flow

```
NewPool(opts...)
  └─ resolve adapter (default: postgres)
  └─ resolve provider (default: postgres-rls)
  └─ store adapter + provider on pool

AddConnection(ctx, connOpts...)
  └─ adapter.NormalizeDSN(dsn)
  └─ adapter.OpenConnection(ctx, dsn, ConnectionOptions{...})
  └─ provider.Wire(adapter, gormDB)   ← registers BeforeAcquire/AfterRelease

Migrate(ctx, dir, models...)
  └─ adapter.AdvisoryLock(ctx, db, MigrationLockID)
  └─ ensureMigrationTable(ctx, migrator)
  └─ db.AutoMigrate(models...)
  └─ enrolled := tenancy.EnrolledModels(db, models)
  └─ provider.Install(ctx, db, enrolled)
  └─ migration patches
```

### `pool.DB(ctx, readOnly)`

Becomes:

```go
func (s *pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
    s.mu.RLock()
    defer s.mu.RUnlock()
    selectedDB := s.selectOne(...)
    if selectedDB == nil { return nil }
    return selectedDB.Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).WithContext(ctx)
}
```

No tx-from-context lookup. No scopes. Tenancy is the adapter's job (via the provider's hooks).

---

## Service / `frame` wiring

`options_datastore.go`:

```go
// WithDatastore is unchanged from a caller perspective. Internally it
// constructs the pool with default dialect + provider.
func WithDatastore(opts ...pool.Option) Option

// WithTenancyProvider overrides the default Postgres-RLS provider.
// Useful for tests or future alternative providers.
func WithTenancyProvider(prov tenancy.Provider) Option
```

`Service`:

```go
// TenancyProvider returns the active tenancy provider for the default
// pool. Used by tests and diagnostics.
func (s *Service) TenancyProvider() tenancy.Provider
```

The Connect interceptor surface:

```go
// In tenancy/interceptor.go:
func NewClaimsInterceptor() connect.Interceptor
```

Usage:

```go
ctx, svc := frame.NewService(
    frame.WithDatastore(),
)

connectInterceptors := connect.WithInterceptors(
    authInterceptor,
    tenancy.NewClaimsInterceptor(),   // cheap; just builds Claims and binds to ctx
    // ...
)
```

The interceptor:
1. Reads `security.ClaimsFromContext(ctx)`.
2. If non-nil, calls `tenancy.ClaimsFromAuth(ctx, auth)` and `tenancy.WithClaims(ctx, claims)`.
3. Calls `next(ctx, req)`.

No transaction. No DB activity. Safe for streaming RPCs.

---

## `data.BaseModel` changes

Add three setters so `BaseModel` satisfies `tenancy.Tenanted`:

```go
func (m *BaseModel) SetTenantID(v string)    { m.TenantID = v }
func (m *BaseModel) SetPartitionID(v string) { m.PartitionID = v }
func (m *BaseModel) SetAccessID(v string)    { m.AccessID = v }
```

The existing `Get*` methods already exist. The existing `BeforeCreate` / `BeforeUpdate` lifecycle hooks are unchanged — they continue to default `TenantID`/`PartitionID` from auth claims at insert time.

---

## Multi-statement atomicity (opt-in, GORM-native)

There is no framework helper for transactions. Callers wanting multi-statement atomicity use GORM directly:

```go
db := pool.DB(ctx, false)
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&e1).Error; err != nil { return err }
    if err := tx.Create(&e2).Error; err != nil { return err }
    return nil
})
```

Properties:

- The tx holds a single connection for its lifetime; that connection was acquired through the provider's `BeforeAcquire`, so all statements in the tx share consistent tenancy scope.
- The `tx *gorm.DB` lives only in the closure's local scope — never in `context.Context`. Cross-system tx propagation is structurally impossible.
- If `fn` returns an error, GORM rolls back and the conn is released → `AfterRelease` resets session state.
- If `fn` returns nil, GORM commits and the conn is released → `AfterRelease` resets session state.

One-shot calls remain the encouraged path: `repo.Create(ctx, entity)`, `repo.GetByID(ctx, id)`, etc. Each one-shot acquires, scopes, queries, releases.

---

## Test strategy

| Test file | Style | Coverage |
|---|---|---|
| `tenancy/claims_test.go` | unit | `Claims.ExtendPartitions` dedup/ordering; `IsEmpty`; `WithExtraPartitions` preserves TenantID; `ClaimsFromContext` fallback chain (explicit → auth-derived → nil); `ClaimsFromAuth` default mapping. |
| `tenancy/postgres/provider_test.go` | testcontainers | `Install` idempotency (run twice — schema stable, no duplicate policies); RLS actually filters cross-tenant rows in real INSERT/SELECT; multi-partition principal sees all their partitions; `Skip=true` claims bypass enforcement; `AfterRelease` resets session state (assert via separate conn that `current_setting('app.tenant_id', true)` is empty after release). |
| `tenancy/interceptor_test.go` | testcontainers | Interceptor binds Claims from auth claims; downstream `pool.DB(ctx, _)` query enforces RLS; missing auth claims → nil Claims → match-all RLS branch. |
| `dialect/postgres/dsn_test.go` | unit | DSN normalisation: URI → libpq form, invalid scheme rejected, query parameters preserved, libpq form passthrough. |
| `dialect/postgres/postgres_test.go` | testcontainers | Advisory lock acquire/release/retry; `IsRelationAlreadyExistsErr` detection; `OpenConnection` wires hooks correctly (register an acquire hook, assert it runs). |
| `datastore/pool/implementation_test.go` | testcontainers (extended) | `DB(ctx, _)` routes reads to replicas / writes to primary; tenancy-aware query through `BaseRepository.Create` + `BaseRepository.GetByID` actually enforces RLS. |
| `datastore/repository_test.go` | existing — testcontainers | Continue to pass unchanged. Critical regression coverage for `BaseRepository`. |

All integration tests reuse `frametests.FrameBaseTestSuite` + `testpostgres` — same pattern already used by `datastore/repository_test.go`.

No fake providers. The tenancy abstraction is proven against real Postgres in every behavioural test.

---

## Robustness & operational properties

| Concern | Mitigation |
|---|---|
| **No tx leaks across systems** | Tx-in-context removed. Transactions live only in the closure passed to `db.Transaction(fn)`. Structurally impossible to leak. |
| **Idempotent install** | Provider's `Install` uses `CREATE OR REPLACE FUNCTION` + `DROP POLICY IF EXISTS … CREATE POLICY` + `ALTER TABLE … FORCE RLS`. Re-running on a configured DB is a no-op. |
| **Connection-pool hook failure** | Hook errors reject the acquire and destroy the conn on release; pgxpool retries with a fresh conn or surfaces a clean error. |
| **Stale session state** | `AfterRelease` issues `RESET app.tenant_id; RESET app.partition_id`. `MaxIdleConns=0` on `sql.DB` guarantees every acquire goes through pgxpool's `BeforeAcquire`. |
| **No-claims (system services, migrations)** | RLS policy's empty-match-all branch handles it — same as today. |
| **Skip claims (internal services)** | Claims hold `Skip=true`; provider's hook is a no-op for the conn → RLS sees empty session vars → match-all. |
| **Override during request** | Caller derives new `Claims` and calls `tenancy.WithClaims(ctx, c)` or `tenancy.WithExtraPartitions(ctx, ids...)`; next acquire reads the new claims. Immutable values → no race. |
| **Tests** | Real Postgres via testcontainers. No fakes. |
| **Multi-pool** | Each `pool.NewPool` gets its own adapter + provider via options. |
| **Extensibility: new DB** | Implement `dialect.DialectAdapter` + `tenancy.Provider`. Register via `WithDialectAdapter` + `WithTenancyProvider`. Pool unchanged. |
| **Extensibility: alt tenancy scheme** | Write a new `Provider`. E.g., a "schema-per-tenant" provider that issues `SET search_path` in `BeforeAcquire`. |
| **Migration safety** | DialectAdapter owns `AdvisoryLock` + `IsRelationAlreadyExistsErr`. Postgres adapter preserves the existing 82548391244719 advisory lock ID + retry semantics. |
| **Telemetry** | Adapter retains `otelpgx`. Provider hooks may attach OTel spans. |
| **Observability of who-saw-what** | `tenancy.ClaimsFromContext(ctx)` is the canonical accessor for audit/log code. |
| **Streaming RPCs** | Claims interceptor is pure-context — no tx held open for the stream. Each message in the stream acquires a conn afresh, scoped from the same Claims in ctx. |

---

## Migration / removal checklist

### Files removed

- `datastore/scopes/tenancy.go`
- `datastore/scopes/` (package directory)
- `datastore/pool/rls.go` (logic moved to `tenancy/postgres/provider.go` + `tenancy/enrollment.go`)
- `security/interceptors/connect/tenancy_tx.go`

### Files heavily edited

- `datastore/pool/interface.go` — strip tenancy methods.
- `datastore/pool/implementation.go` — remove tenancy code, plumb `dialect.DialectAdapter` + `tenancy.Provider`.
- `datastore/pool/connection.go` — gut Postgres specifics; delegate to adapter.
- `datastore/pool/options.go` — add `WithDialectAdapter`, `WithTenancyProvider`.
- `data/model.go` — add three setters for `tenancy.Tenanted`.
- `options_datastore.go` — wire default Postgres adapter + provider; add `WithTenancyProvider`; add `svc.TenancyProvider()` accessor.
- `docs/datastore.md` — rewrite tenancy section pointing at `tenancy/` package.

### Files added

- `tenancy/claims.go`
- `tenancy/provider.go`
- `tenancy/marker.go`
- `tenancy/enrollment.go`
- `tenancy/interceptor.go`
- `tenancy/claims_test.go`
- `tenancy/interceptor_test.go`
- `tenancy/postgres/provider.go`
- `tenancy/postgres/sql.go`
- `tenancy/postgres/provider_test.go`
- `datastore/dialect/adapter.go`
- `datastore/dialect/postgres/postgres.go`
- `datastore/dialect/postgres/dsn.go`
- `datastore/dialect/postgres/dsn_test.go`
- `datastore/dialect/postgres/postgres_test.go`

### Callers requiring update outside frame

Downstream services that today call:

- `pool.WithRequestTx(ctx, fn)` → replace with one-shot `pool.DB(ctx, _).…` calls, or explicit `db.Transaction(fn)` for atomicity.
- `pool.WithTenancy(ctx, readOnly, fn)` → same.
- `connect_interceptors.NewTenancyTxInterceptor(pool)` → replace with `tenancy.NewClaimsInterceptor()`.

A short note will be added to `docs/datastore.md` explaining the migration.

---

## Open questions

None at design time. Anything that surfaces during implementation will be raised in the implementation plan.

---

## Acceptance criteria

1. All packages listed in "Files added" exist with full unit + integration tests.
2. `datastore/scopes/` is deleted.
3. `Pool` interface has no tenancy methods.
4. `tenancy.NewClaimsInterceptor()` replaces `connect_interceptors.NewTenancyTxInterceptor`.
5. `data.BaseModel` implements `tenancy.Tenanted`.
6. All existing tests in `datastore/repository_test.go` continue to pass.
7. `go test -race ./...` is green.
8. `golangci-lint run` is green.
9. RLS enforcement verified end-to-end in `tenancy/postgres/provider_test.go` against real Postgres.
10. The Postgres advisory lock for migrations preserves the existing ID (82548391244719) and retry behaviour.
