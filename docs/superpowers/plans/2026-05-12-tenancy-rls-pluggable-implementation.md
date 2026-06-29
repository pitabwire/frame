# Tenancy & RLS Pluggable — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Postgres-coupled RLS code in `datastore/pool` with two narrow, swappable abstractions (`tenancy.Provider`, `dialect.DialectAdapter`); remove `datastore/scopes` and all tx-in-context plumbing; introduce a dedicated immutable `tenancy.Claims` value object derived from auth claims.

**Architecture:** A new top-level `tenancy/` package owns the immutable `Claims` value, the `Provider` interface, enrollment detection, and a lightweight Connect interceptor that binds Claims to context. A new `datastore/dialect/` package owns driver concerns (DSN parsing, pgxpool/GORM construction, advisory locking, connection-acquire hook registration). The `datastore/pool` package is refactored to compose these two abstractions and drops every tenancy method. Enforcement happens at connection acquire — pgxpool's `BeforeAcquire` calls the Postgres tenancy provider's hook, which emits `SELECT set_config('app.tenant_id', $1, false), set_config('app.partition_id', $2, false)` derived from `tenancy.ClaimsFromContext(ctx)`. Transactions never live in `context.Context`; multi-statement atomicity is opt-in via GORM's `db.Transaction(fn)`.

**Tech Stack:** Go 1.23, GORM v2, pgx/v5, pgxpool, otelpgx, Connect RPC, testify/suite, testcontainers via `frametests.FrameBaseTestSuite` + `testpostgres`.

**Spec:** [`docs/superpowers/specs/2026-05-12-tenancy-rls-pluggable-design.md`](../specs/2026-05-12-tenancy-rls-pluggable-design.md)

**Refinement of spec:** The spec defines a single `Provider.Wire(adapter, db)` method. The plan splits this into `WireAdapter(adapter dialect.DialectAdapter) error` and `WireGorm(db *gorm.DB) error` so the timing is explicit: `WireAdapter` runs at pool construction (before any connection is opened, so hooks attach to subsequently-created pgxpools); `WireGorm` runs per opened connection. Postgres-RLS implements `WireGorm` as a no-op since it operates at the connection-acquire level. No other behaviour changes.

---

## File map

### New files
- `tenancy/claims.go`
- `tenancy/claims_test.go`
- `tenancy/marker.go`
- `tenancy/provider.go`
- `tenancy/enrollment.go`
- `tenancy/enrollment_test.go`
- `tenancy/interceptor.go`
- `tenancy/interceptor_test.go`
- `tenancy/postgres/provider.go`
- `tenancy/postgres/sql.go`
- `tenancy/postgres/provider_test.go`
- `datastore/dialect/adapter.go`
- `datastore/dialect/postgres/dsn.go`
- `datastore/dialect/postgres/dsn_test.go`
- `datastore/dialect/postgres/postgres.go`
- `datastore/dialect/postgres/postgres_test.go`

### Modified files
- `data/model.go` — add `SetTenantID`, `SetPartitionID`, `SetAccessID`
- `datastore/pool/interface.go` — strip tenancy methods
- `datastore/pool/options.go` — add `WithDialectAdapter`, `WithTenancyProvider`
- `datastore/pool/connection.go` — delegate to adapter
- `datastore/pool/implementation.go` — remove tenancy/scopes; use adapter + provider
- `options_datastore.go` — wire default adapter+provider; add `WithTenancyProvider`; add `svc.TenancyProvider()`
- `service.go` — add `tenancyProvider` field + accessor
- `docs/datastore.md` — rewrite tenancy section

### Deleted files
- `datastore/pool/rls.go`
- `datastore/scopes/tenancy.go`
- `datastore/scopes/` (package directory)
- `security/interceptors/connect/tenancy_tx.go`

---

## Conventions used throughout this plan

- **Imports** are written in goimports order (stdlib, blank, third-party, local with internal grouping).
- **Test files** colocate under the same package unless stated otherwise.
- **Run tests with:** `go test -race ./...` (selective per-package commands are given in each task).
- **Lint:** `golangci-lint run` after structural changes; do not introduce new lint diagnostics.
- **Commits** use the `feat(<scope>):` / `refactor(<scope>):` / `test(<scope>):` / `docs(<scope>):` convention seen in `git log`.

---

## Task 1: Tenancy package skeleton — markers

**Files:**
- Create: `tenancy/marker.go`

- [ ] **Step 1: Write the marker file**

```go
// Package tenancy provides the storage-layer view of a principal's
// tenancy (Claims), the Provider abstraction for enforcement, and
// helpers to bind tenancy to a request context. The package is
// intentionally narrow: it depends only on security (for the default
// auth-claims derivation) and gorm.io/gorm.
package tenancy

// Tenanted is the structural interface a model must satisfy to be
// enrolled in tenancy enforcement. data.BaseModel satisfies it; custom
// models that want enrollment can satisfy it explicitly.
//
// The tenancy package never imports the data package — enrollment is
// purely structural, so downstream services can roll their own
// tenanted base type if needed.
type Tenanted interface {
	GetTenantID() string
	GetPartitionID() string
	GetAccessID() string
	SetTenantID(string)
	SetPartitionID(string)
	SetAccessID(string)
}

// Unscoped opts a model out of tenancy enforcement. Implement this
// interface to skip RLS policy installation for the model's table.
// The canonical way to satisfy it is to embed UnscopedMarker:
//
//	type LookupTable struct {
//	    ID string
//	    tenancy.UnscopedMarker
//	}
type Unscoped interface {
	TenancyUnscoped()
}

// UnscopedMarker is an empty struct satisfying Unscoped. Embed it in
// a model to opt out of tenancy enforcement.
type UnscopedMarker struct{}

// TenancyUnscoped implements Unscoped.
func (UnscopedMarker) TenancyUnscoped() {}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./tenancy/...`
Expected: success (no other files yet, but the package compiles).

- [ ] **Step 3: Commit**

```bash
git add tenancy/marker.go
git commit -m "feat(tenancy): introduce Tenanted and Unscoped markers"
```

---

## Task 2: Claims value object — core type + IsEmpty + ExtendPartitions

**Files:**
- Create: `tenancy/claims.go`
- Create: `tenancy/claims_test.go`

- [ ] **Step 1: Write the failing test**

Create `tenancy/claims_test.go`:

```go
package tenancy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/v2/tenancy"
)

func TestClaimsIsEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, (&tenancy.Claims{}).IsEmpty())
	require.True(t, (&tenancy.Claims{Skip: true}).IsEmpty(),
		"Skip alone does not carry tenancy")
	require.False(t, (&tenancy.Claims{TenantID: "t1"}).IsEmpty())
	require.False(t, (&tenancy.Claims{PartitionIDs: []string{"p1"}}).IsEmpty())
}

func TestExtendPartitionsDedupAndOrder(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1", "p2"}, AccessID: "a1", Skip: false}
	extended := base.ExtendPartitions("p3", "p2", "p4")

	require.NotSame(t, base, extended, "ExtendPartitions must return a new instance")
	require.Equal(t, []string{"p1", "p2"}, base.PartitionIDs, "base must be unchanged")
	require.Equal(t, []string{"p1", "p2", "p3", "p4"}, extended.PartitionIDs)
	require.Equal(t, "t1", extended.TenantID)
	require.Equal(t, "a1", extended.AccessID)
	require.False(t, extended.Skip)
}

func TestExtendPartitionsPreservesSkip(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", Skip: true}
	extended := base.ExtendPartitions("p1")
	require.True(t, extended.Skip, "Skip must be preserved across extension")
}

func TestExtendPartitionsNoOpWhenAllPresent(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1", "p2"}}
	extended := base.ExtendPartitions("p1", "p2")
	require.Equal(t, base.PartitionIDs, extended.PartitionIDs)
}

func TestExtendPartitionsIgnoresEmpty(t *testing.T) {
	t.Parallel()

	base := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1"}}
	extended := base.ExtendPartitions("", "p2", "")
	require.Equal(t, []string{"p1", "p2"}, extended.PartitionIDs)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tenancy/ -run TestClaims -run TestExtendPartitions`
Expected: FAIL — `Claims` undefined.

- [ ] **Step 3: Write the implementation**

Create `tenancy/claims.go`:

```go
package tenancy

// Claims is the storage-layer view of a principal's tenancy. Treat as
// immutable: every transformation returns a new instance.
type Claims struct {
	// TenantID is the single tenant this principal belongs to.
	TenantID string

	// PartitionIDs are every partition this principal can access. One
	// principal may legitimately span multiple partitions (e.g., an
	// operator with access to several branches, an analyst aggregating
	// across groups). Single-partition principals carry one element.
	PartitionIDs []string

	// AccessID is an optional access-control hint propagated through
	// queue metadata and lifecycle hooks.
	AccessID string

	// Skip is true for internal/system callers that should bypass
	// tenancy enforcement. Providers honour Skip by performing no
	// session binding for the conn — the database-side policy's
	// empty-match-all branch then keeps every row visible.
	Skip bool
}

// IsEmpty reports whether the claims carry enforceable tenancy. Empty
// claims behave identically to "no claims attached" from a provider's
// perspective.
func (c *Claims) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.TenantID == "" && len(c.PartitionIDs) == 0
}

// ExtendPartitions returns a new Claims with the supplied partition IDs
// merged in. Preserves TenantID, AccessID, and Skip unchanged. Empty
// strings are ignored; duplicates are removed; existing order is kept
// and new IDs appended after.
func (c *Claims) ExtendPartitions(partitionIDs ...string) *Claims {
	if c == nil {
		return &Claims{PartitionIDs: dedupedNonEmpty(partitionIDs)}
	}

	merged := make([]string, 0, len(c.PartitionIDs)+len(partitionIDs))
	seen := make(map[string]struct{}, cap(merged))
	for _, p := range c.PartitionIDs {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	for _, p := range partitionIDs {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}

	return &Claims{
		TenantID:     c.TenantID,
		PartitionIDs: merged,
		AccessID:     c.AccessID,
		Skip:         c.Skip,
	}
}

func dedupedNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./tenancy/ -v`
Expected: PASS — TestClaimsIsEmpty, TestExtendPartitionsDedupAndOrder, TestExtendPartitionsPreservesSkip, TestExtendPartitionsNoOpWhenAllPresent, TestExtendPartitionsIgnoresEmpty all pass.

- [ ] **Step 5: Commit**

```bash
git add tenancy/claims.go tenancy/claims_test.go
git commit -m "feat(tenancy): add immutable Claims with additive ExtendPartitions"
```

---

## Task 3: BaseModel implements Tenanted

**Files:**
- Modify: `data/model.go` (around the existing `Get*` methods)

- [ ] **Step 1: Add the setters to BaseModel**

Open `data/model.go`. After the existing `GetAccessID` method (around line 97), add:

```go
// SetTenantID assigns the tenant identifier. Satisfies tenancy.Tenanted.
func (model *BaseModel) SetTenantID(v string) {
	model.TenantID = v
}

// SetPartitionID assigns the partition identifier. Satisfies tenancy.Tenanted.
func (model *BaseModel) SetPartitionID(v string) {
	model.PartitionID = v
}

// SetAccessID assigns the access identifier. Satisfies tenancy.Tenanted.
func (model *BaseModel) SetAccessID(v string) {
	model.AccessID = v
}
```

- [ ] **Step 2: Write a compile-time interface check**

In `data/model.go`, after the new setters, add the compile-time assertion. The data package must NOT import `tenancy` (would create a cycle once the tenancy package eventually needs the marker check). Instead, declare a local interface mirror and assert against it:

```go
// tenancyTenantedMirror mirrors tenancy.Tenanted's method set so we can
// assert BaseModel satisfies it without importing the tenancy package
// (which would cycle: tenancy depends on security, data depends on
// security, future code may have tenancy depend on data).
type tenancyTenantedMirror interface {
	GetTenantID() string
	GetPartitionID() string
	GetAccessID() string
	SetTenantID(string)
	SetPartitionID(string)
	SetAccessID(string)
}

var _ tenancyTenantedMirror = (*BaseModel)(nil)
```

- [ ] **Step 3: Run existing data tests**

Run: `go test ./data/... -v`
Expected: PASS — existing tests unchanged; new compile-time assertion compiles.

- [ ] **Step 4: Run a structural check from a new test**

Add to `data/model_test.go` (create the test inside the existing file at the end):

```go
func TestBaseModelSettersRoundTrip(t *testing.T) {
	t.Parallel()

	m := &data.BaseModel{}
	m.SetTenantID("t1")
	m.SetPartitionID("p1")
	m.SetAccessID("a1")
	require.Equal(t, "t1", m.GetTenantID())
	require.Equal(t, "p1", m.GetPartitionID())
	require.Equal(t, "a1", m.GetAccessID())
}
```

If `data/model_test.go` doesn't already exist, create it with the standard package + imports:

```go
package data_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/v2/data"
)
```

Run: `go test ./data/... -run TestBaseModelSettersRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add data/model.go data/model_test.go
git commit -m "feat(data): BaseModel implements tenancy.Tenanted via setters"
```

---

## Task 4: Tenancy provider interface

**Files:**
- Create: `tenancy/provider.go`

- [ ] **Step 1: Write the provider interface**

Create `tenancy/provider.go`:

```go
package tenancy

import (
	"context"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
)

// Provider installs and enforces tenancy isolation at the storage
// layer. Implementations are database-specific; the bundled Postgres
// provider uses Row-Level Security policies, others might use views,
// query rewriting, or a different scheme entirely.
type Provider interface {
	// Name returns a short, stable identifier ("postgres-rls") used in
	// logs and diagnostics.
	Name() string

	// Capabilities advertises what the provider does so the pool can
	// decide whether a complementary fallback (e.g., GORM scope) is
	// required.
	Capabilities() Capabilities

	// Install applies storage-side enforcement schema (RLS policies,
	// views, etc.) for the supplied models. Called once per migration.
	// Implementations MUST be idempotent — Frame re-runs migration on
	// every boot.
	Install(ctx context.Context, db *gorm.DB, models []ModelInfo) error

	// WireAdapter registers dialect-level hooks. Called once when the
	// pool is constructed, BEFORE any connection is opened. Providers
	// that enforce per-acquire (Postgres-RLS) register here.
	WireAdapter(adapter dialect.DialectAdapter) error

	// WireGorm registers GORM-level callbacks on the supplied *gorm.DB.
	// Called once per opened connection. Providers that enforce
	// per-query (alternative dialects without per-acquire hooks)
	// register here. Postgres-RLS implements as a no-op.
	WireGorm(db *gorm.DB) error
}

// Capabilities describes the runtime behaviour of a Provider.
type Capabilities struct {
	// EnforcesAtStorage is true when the provider installs DB-side
	// rules that block access without per-query gating (e.g., RLS,
	// views). Used by the pool to skip any fallback scope it might
	// otherwise have applied.
	EnforcesAtStorage bool
}

// ModelInfo describes one tenancy-enrolled model for Install. Built by
// the tenancy package via reflective enrollment; providers don't
// reimplement detection.
type ModelInfo struct {
	// Table is the SQL table name resolved through GORM's naming
	// strategy.
	Table string

	// TenantColumn is the SQL column carrying the tenant identifier.
	// Default "tenant_id".
	TenantColumn string

	// PartitionColumn is the SQL column carrying the partition
	// identifier. Default "partition_id".
	PartitionColumn string
}
```

- [ ] **Step 2: Verify compilation fails for the dialect import (no package yet)**

Run: `go build ./tenancy/...`
Expected: FAIL — `github.com/pitabwire/frame/v2/datastore/dialect` not found.

This is expected. Skip ahead to Task 5 which introduces the dialect package; we'll loop back to compile this.

- [ ] **Step 3: Commit the provider file as-is**

```bash
git add tenancy/provider.go
git commit -m "feat(tenancy): declare Provider interface (forward-references dialect)"
```

Note: this commit will not build standalone; it's resolved by Task 5. If the executing engineer requires every commit to build, swap the order of Tasks 4 and 5 — they are otherwise independent.

---

## Task 5: Dialect package — interface skeleton

**Files:**
- Create: `datastore/dialect/adapter.go`

- [ ] **Step 1: Write the adapter interface**

Create `datastore/dialect/adapter.go`:

```go
// Package dialect defines the driver abstraction frame uses to talk to
// a database. A DialectAdapter is responsible for DSN normalisation,
// connection construction (gorm.Dialector + *sql.DB), advisory locking
// for migrations, identifier quoting, and registering per-connection
// hooks that providers (e.g., tenancy.Provider) attach to inject
// per-request state on connection acquire / release.
package dialect

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DialectAdapter abstracts every database-specific concern in the
// frame datastore layer. Implementations are concrete per database
// (postgres, mysql, sqlite); the pool composes one adapter with one
// tenancy provider.
type DialectAdapter interface {
	// Name returns a short, stable identifier ("postgres") used in
	// logs.
	Name() string

	// NormalizeDSN converts URI or libpq form into the dialect's
	// expected DSN. Returns the normalized string and an error if the
	// input cannot be parsed.
	NormalizeDSN(raw string) (string, error)

	// OpenConnection constructs a gorm.Dialector + underlying *sql.DB
	// for the supplied DSN. The adapter is responsible for wiring
	// observability (e.g. otelpgx) and tuning (pool sizes, lifetimes).
	// Registered hooks (RegisterAcquireHook / RegisterReleaseHook) must
	// be attached to the connection that is opened.
	OpenConnection(
		ctx context.Context,
		dsn string,
		opts ConnectionOptions,
	) (gorm.Dialector, *sql.DB, error)

	// AdvisoryLock acquires a cooperative lock for serializing
	// migrations. Returns a release function (nil release means lock
	// acquisition is not supported by this dialect, in which case the
	// caller should warn and proceed).
	AdvisoryLock(ctx context.Context, db *gorm.DB, id int64) (release func(), err error)

	// IsRelationAlreadyExistsErr discriminates "table already exists"
	// errors that occur during concurrent migration startup races.
	IsRelationAlreadyExistsErr(err error) bool

	// QuoteIdentifier returns the supplied identifier quoted in the
	// dialect's native form. Used by providers when emitting DDL.
	QuoteIdentifier(name string) string

	// RegisterAcquireHook adds a callback invoked on every connection
	// acquire from the underlying pool. Hooks are called in
	// registration order. Returning an error rejects the acquire.
	//
	// Must be called BEFORE OpenConnection — hooks registered after
	// connection construction are not retroactively attached.
	RegisterAcquireHook(fn AcquireHook) error

	// RegisterReleaseHook adds a callback invoked on every connection
	// release. Hooks are called in registration order. Returning an
	// error causes the conn to be destroyed instead of returned to the
	// pool — useful for poisoned state detection.
	RegisterReleaseHook(fn ReleaseHook) error
}

// AcquireHook is invoked before a connection is handed out from the
// pool. Receives the context carried into the acquire call (which is
// the context the caller used on the topmost db.QueryContext) and a
// DialectConn wrapper around the native connection.
type AcquireHook func(ctx context.Context, conn DialectConn) error

// ReleaseHook is invoked before a connection is returned to the pool.
type ReleaseHook func(ctx context.Context, conn DialectConn) error

// DialectConn is the minimal surface a hook needs. Each adapter wraps
// its native connection (e.g. *pgx.Conn) behind this interface so no
// driver types leak past the hook boundary.
type DialectConn interface {
	// Exec runs a parameterised statement on the connection without
	// returning rows. Used by hooks to issue session-binding SQL.
	Exec(ctx context.Context, query string, args ...any) error
}

// ConnectionOptions tune the connection pool. Mirrors the previously
// per-dialect Options fields so callers see one consistent shape.
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

- [ ] **Step 2: Verify build**

Run: `go build ./datastore/dialect/... ./tenancy/...`
Expected: success. Both packages compile (tenancy now finds dialect).

- [ ] **Step 3: Commit**

```bash
git add datastore/dialect/adapter.go
git commit -m "feat(dialect): introduce DialectAdapter abstraction"
```

---

## Task 6: Postgres dialect — DSN normalisation (unit)

**Files:**
- Create: `datastore/dialect/postgres/dsn.go`
- Create: `datastore/dialect/postgres/dsn_test.go`

- [ ] **Step 1: Write the failing test**

Create `datastore/dialect/postgres/dsn_test.go`:

```go
package postgres_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/v2/datastore/dialect/postgres"
)

func TestNormalizeDSNLibpqPassthrough(t *testing.T) {
	t.Parallel()

	in := "host=localhost port=5432 user=u password=p dbname=test"
	out, err := postgres.NormalizeDSN(in)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestNormalizeDSNURIConversion(t *testing.T) {
	t.Parallel()

	out, err := postgres.NormalizeDSN("postgres://u:p@localhost:5432/test?sslmode=disable")
	require.NoError(t, err)
	require.Contains(t, out, "host=localhost")
	require.Contains(t, out, "port=5432")
	require.Contains(t, out, "user=u")
	require.Contains(t, out, "password=p")
	require.Contains(t, out, "dbname=test")
	require.Contains(t, out, "sslmode=disable")
}

func TestNormalizeDSNDefaultPort(t *testing.T) {
	t.Parallel()

	out, err := postgres.NormalizeDSN("postgres://u:p@localhost/test")
	require.NoError(t, err)
	require.Contains(t, out, "port=5432")
}

func TestNormalizeDSNRejectsBadScheme(t *testing.T) {
	t.Parallel()

	_, err := postgres.NormalizeDSN("mysql://u:p@localhost/test")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid scheme"),
		"expected invalid scheme error, got: %v", err)
}

func TestNormalizeDSNAcceptsPostgresql(t *testing.T) {
	t.Parallel()

	out, err := postgres.NormalizeDSN("postgresql://u:p@localhost/test")
	require.NoError(t, err)
	require.Contains(t, out, "dbname=test")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./datastore/dialect/postgres/... -run TestNormalizeDSN`
Expected: FAIL — `postgres.NormalizeDSN` undefined.

- [ ] **Step 3: Write the implementation**

Create `datastore/dialect/postgres/dsn.go`:

```go
// Package postgres provides the Postgres concrete DialectAdapter.
package postgres

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeDSN converts a postgres:// or postgresql:// URI into the
// libpq keyword=value DSN form. If the input already looks like libpq
// form (contains '=' and no postgres scheme prefix) it is returned
// unchanged.
//
// Returns an error if the URI scheme is not postgres / postgresql.
func NormalizeDSN(pgString string) (string, error) {
	trimmed := strings.TrimSpace(pgString)
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "=") && !strings.HasPrefix(lower, "postgres://") &&
		!strings.HasPrefix(lower, "postgresql://") {
		return trimmed, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	dbname := strings.TrimPrefix(u.Path, "/")

	parts := []string{
		"host=" + host,
		"port=" + port,
		"user=" + user,
		"password=" + password,
		"dbname=" + dbname,
	}
	for k, vals := range u.Query() {
		for _, v := range vals {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(parts, " "), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./datastore/dialect/postgres/... -run TestNormalizeDSN -v`
Expected: PASS — all five DSN test cases.

- [ ] **Step 5: Commit**

```bash
git add datastore/dialect/postgres/dsn.go datastore/dialect/postgres/dsn_test.go
git commit -m "feat(dialect/postgres): port DSN normalisation"
```

---

## Task 7: Postgres dialect — adapter skeleton with hook registry

**Files:**
- Create: `datastore/dialect/postgres/postgres.go`

- [ ] **Step 1: Write the adapter (hook registration + identifier quoting + relation-error detection)**

This task creates the adapter struct and the lifecycle methods that don't need a live database. The connection-opening + advisory lock are exercised by integration tests in Task 8.

Create `datastore/dialect/postgres/postgres.go`:

```go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
)

const idleTimeToMaxLifeTimeDivisor = 2
const migrationLockRetryInterval = 200 * time.Millisecond

// Adapter is the Postgres concrete implementation of
// dialect.DialectAdapter. Construct with New; hook registration must
// happen BEFORE OpenConnection.
type Adapter struct {
	mu            sync.RWMutex
	acquireHooks  []dialect.AcquireHook
	releaseHooks  []dialect.ReleaseHook
}

// New returns a fresh Adapter with no registered hooks.
func New() *Adapter {
	return &Adapter{}
}

// Name implements dialect.DialectAdapter.
func (*Adapter) Name() string { return "postgres" }

// NormalizeDSN implements dialect.DialectAdapter.
func (*Adapter) NormalizeDSN(raw string) (string, error) {
	return NormalizeDSN(raw)
}

// QuoteIdentifier implements dialect.DialectAdapter.
func (*Adapter) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// IsRelationAlreadyExistsErr implements dialect.DialectAdapter.
// Detects SQLSTATE 42P07 ("relation already exists"), used to gracefully
// handle concurrent migration startup races.
func (*Adapter) IsRelationAlreadyExistsErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P07"
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// RegisterAcquireHook implements dialect.DialectAdapter.
func (a *Adapter) RegisterAcquireHook(fn dialect.AcquireHook) error {
	if fn == nil {
		return errors.New("dialect/postgres: nil acquire hook")
	}
	a.mu.Lock()
	a.acquireHooks = append(a.acquireHooks, fn)
	a.mu.Unlock()
	return nil
}

// RegisterReleaseHook implements dialect.DialectAdapter.
func (a *Adapter) RegisterReleaseHook(fn dialect.ReleaseHook) error {
	if fn == nil {
		return errors.New("dialect/postgres: nil release hook")
	}
	a.mu.Lock()
	a.releaseHooks = append(a.releaseHooks, fn)
	a.mu.Unlock()
	return nil
}

// snapshotHooks returns immutable copies under read-lock so callbacks
// invoked from pgxpool see a stable hook chain even if RegisterX is
// called concurrently (which is rare but legal).
func (a *Adapter) snapshotHooks() ([]dialect.AcquireHook, []dialect.ReleaseHook) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	acq := append([]dialect.AcquireHook(nil), a.acquireHooks...)
	rel := append([]dialect.ReleaseHook(nil), a.releaseHooks...)
	return acq, rel
}

// pgxDialectConn wraps a *pgxpool.Conn so it satisfies
// dialect.DialectConn without leaking pgx types past the boundary.
type pgxDialectConn struct {
	c *pgxpool.Conn
}

func (w *pgxDialectConn) Exec(ctx context.Context, query string, args ...any) error {
	_, err := w.c.Exec(ctx, query, args...)
	return err
}

// OpenConnection implements dialect.DialectAdapter.
//
// Pool sizing notes:
//   - MaxIdleConns is forced to 0 on the *sql.DB so every release goes
//     through pgxpool — this is the property the hook chain relies on
//     to guarantee BeforeAcquire fires per query, never leaking
//     session state between requests.
//   - MaxOpenConns mirrors pgxpool.MaxConns so sql.DB never tries to
//     open more conns than the pool allows.
func (a *Adapter) OpenConnection(
	ctx context.Context,
	dsn string,
	opts dialect.ConnectionOptions,
) (gorm.Dialector, *sql.DB, error) {
	cleanDSN, err := a.NormalizeDSN(dsn)
	if err != nil {
		return nil, nil, err
	}

	cfg, err := pgxpool.ParseConfig(cleanDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("create connection pool: %w", err)
	}

	if opts.MaxOpen > 0 {
		maxConns := opts.MaxOpen
		if maxConns > math.MaxInt32 {
			maxConns = math.MaxInt32
		}
		cfg.MaxConns = int32(maxConns)
	}
	if opts.MaxLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxLifetime
		cfg.MaxConnIdleTime = opts.MaxLifetime / idleTimeToMaxLifeTimeDivisor
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()

	// Wire BeforeAcquire / AfterRelease to dispatch through registered
	// hooks. Hooks see a stable snapshot — concurrent RegisterX after
	// construction won't disturb in-flight callbacks.
	cfg.BeforeAcquire = func(hookCtx context.Context, conn *pgxpool.Conn) bool {
		acq, _ := a.snapshotHooks()
		wrapped := &pgxDialectConn{c: conn}
		for _, h := range acq {
			if hookErr := h(hookCtx, wrapped); hookErr != nil {
				return false
			}
		}
		return true
	}
	cfg.AfterRelease = func(conn *pgxpool.Conn) bool {
		_, rel := a.snapshotHooks()
		if len(rel) == 0 {
			return true
		}
		wrapped := &pgxDialectConn{c: conn}
		// pgxpool's AfterRelease has no ctx; use Background to keep
		// reset SQL from being cancelled by an already-cancelled
		// request ctx. Hooks must remain cheap.
		hookCtx := context.Background()
		for _, h := range rel {
			if hookErr := h(hookCtx, wrapped); hookErr != nil {
				return false
			}
		}
		return true
	}

	pgxPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	if statErr := otelpgx.RecordStats(pgxPool); statErr != nil {
		return nil, nil, fmt.Errorf("unable to record database stats: %w", statErr)
	}

	connector := stdlib.GetPoolConnector(pgxPool)
	sqlDB := sql.OpenDB(connector)
	sqlDB.SetMaxIdleConns(0)
	if opts.MaxOpen > 0 {
		sqlDB.SetMaxOpenConns(opts.MaxOpen)
	}
	if opts.MaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(opts.MaxLifetime)
	}

	dialector := gormpostgres.New(gormpostgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: opts.PreferSimpleProtocol,
	})

	return dialector, sqlDB, nil
}

// AdvisoryLock implements dialect.DialectAdapter using Postgres
// pg_try_advisory_lock semantics. Retries every migrationLockRetryInterval
// until the supplied ctx is cancelled or the lock is acquired.
func (a *Adapter) AdvisoryLock(ctx context.Context, db *gorm.DB, id int64) (func(), error) {
	if db == nil {
		return nil, errors.New("dialect/postgres: nil db")
	}

	ticker := time.NewTicker(migrationLockRetryInterval)
	defer ticker.Stop()

	for {
		var acquired bool
		err := db.WithContext(ctx).
			Raw("SELECT pg_try_advisory_lock(?)", id).
			Scan(&acquired).Error
		if err != nil {
			return nil, err
		}
		if acquired {
			return func() {
				unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = db.WithContext(unlockCtx).
					Exec("SELECT pg_advisory_unlock(?)", id).Error
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ dialect.DialectAdapter = (*Adapter)(nil)
```

- [ ] **Step 2: Verify build**

Run: `go build ./datastore/dialect/...`
Expected: success.

- [ ] **Step 3: Run the existing DSN unit tests**

Run: `go test ./datastore/dialect/postgres/... -v`
Expected: PASS — DSN tests still pass, adapter compiles.

- [ ] **Step 4: Commit**

```bash
git add datastore/dialect/postgres/postgres.go
git commit -m "feat(dialect/postgres): Adapter with pgxpool hook plumbing"
```

---

## Task 8: Postgres dialect — integration tests (testcontainer)

**Files:**
- Create: `datastore/dialect/postgres/postgres_test.go`

- [ ] **Step 1: Write the failing integration tests**

Create `datastore/dialect/postgres/postgres_test.go`:

```go
package postgres_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/datastore/dialect/postgres"
	"github.com/pitabwire/frame/v2/frametests/definition"
	"github.com/pitabwire/frame/v2/tests"
)

type AdapterTestSuite struct {
	tests.BaseTestSuite
}

func TestAdapterSuite(t *testing.T) {
	suite.Run(t, &AdapterTestSuite{})
}

func (s *AdapterTestSuite) TestQuoteIdentifierEscapesEmbeddedQuotes() {
	a := postgres.New()
	require.Equal(s.T(), `"table"`, a.QuoteIdentifier("table"))
	require.Equal(s.T(), `"tab""le"`, a.QuoteIdentifier(`tab"le`))
}

func (s *AdapterTestSuite) TestIsRelationAlreadyExistsErrDetection() {
	a := postgres.New()
	require.True(s.T(), a.IsRelationAlreadyExistsErr(&pgconn.PgError{Code: "42P07"}))
	require.True(s.T(), a.IsRelationAlreadyExistsErr(errors.New(`relation "x" already exists`)))
	require.False(s.T(), a.IsRelationAlreadyExistsErr(&pgconn.PgError{Code: "23505"}))
	require.False(s.T(), a.IsRelationAlreadyExistsErr(nil))
}

func (s *AdapterTestSuite) TestOpenConnectionRunsAcquireHook() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		a := postgres.New()
		var acquired int32
		require.NoError(t, a.RegisterAcquireHook(func(_ context.Context, _ dialect.DialectConn) error {
			atomic.AddInt32(&acquired, 1)
			return nil
		}))

		_, sqlDB, err := a.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 2})
		require.NoError(t, err)
		defer func() { _ = sqlDB.Close() }()

		row := sqlDB.QueryRowContext(ctx, "SELECT 1")
		var got int
		require.NoError(t, row.Scan(&got))
		require.Equal(t, 1, got)

		// At least one acquire happened to serve SELECT 1.
		require.GreaterOrEqual(t, atomic.LoadInt32(&acquired), int32(1))
	})
}

func (s *AdapterTestSuite) TestAcquireHookFailureDropsConn() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		a := postgres.New()
		require.NoError(t, a.RegisterAcquireHook(func(_ context.Context, _ dialect.DialectConn) error {
			return errors.New("boom")
		}))

		_, sqlDB, err := a.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 1})
		require.NoError(t, err)
		defer func() { _ = sqlDB.Close() }()

		// With MaxOpen=1 and BeforeAcquire returning false, pgxpool keeps
		// rejecting and queries should fail.
		err = sqlDB.QueryRowContext(ctx, "SELECT 1").Scan(new(int))
		require.Error(t, err)
	})
}

func (s *AdapterTestSuite) TestAdvisoryLockAcquireAndRelease() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		a := postgres.New()
		dialector, sqlDB, err := a.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 4})
		require.NoError(t, err)
		defer func() { _ = sqlDB.Close() }()

		db, err := openGORM(dialector)
		require.NoError(t, err)

		release, err := a.AdvisoryLock(ctx, db, 999_888_777)
		require.NoError(t, err)
		require.NotNil(t, release)
		release()

		// Re-acquire after release succeeds.
		release2, err := a.AdvisoryLock(ctx, db, 999_888_777)
		require.NoError(t, err)
		release2()
	})
}
```

Add the GORM imports to the imports block at the top of the file:

```go
import (
    // ... existing imports ...
    "gorm.io/gorm"
)
```

The `TestAdvisoryLockAcquireAndRelease` test directly calls `gorm.Open(dialector, &gorm.Config{})` to materialise a `*gorm.DB` from the adapter-returned dialector — no separate helper is needed. The relevant lines in that test become:

```go
db, err := gorm.Open(dialector, &gorm.Config{})
require.NoError(t, err)

release, err := a.AdvisoryLock(ctx, db, 999_888_777)
```

- [ ] **Step 2: Run the integration tests**

Run: `go test ./datastore/dialect/postgres/... -run TestAdapterSuite -v`
Expected: PASS for all five subtests (QuoteIdentifier, IsRelationAlreadyExistsErr, OpenConnectionRunsAcquireHook, AcquireHookFailureDropsConn, AdvisoryLockAcquireAndRelease).

Note: this test boots a Postgres testcontainer; the first run downloads the image. Subsequent runs are fast.

- [ ] **Step 3: Commit**

```bash
git add datastore/dialect/postgres/postgres_test.go
git commit -m "test(dialect/postgres): integration coverage for hooks + advisory lock"
```

---

## Task 9: Tenancy enrollment — model detection

**Files:**
- Create: `tenancy/enrollment.go`
- Create: `tenancy/enrollment_test.go`

- [ ] **Step 1: Write the failing test**

Create `tenancy/enrollment_test.go`:

```go
package tenancy_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/pitabwire/frame/v2/tenancy"
)

// fakeTenanted satisfies tenancy.Tenanted minimally for the unit test.
type fakeTenanted struct {
	ID          string `gorm:"primaryKey"`
	TenantID    string
	PartitionID string
	AccessID    string
}

func (f *fakeTenanted) GetTenantID() string    { return f.TenantID }
func (f *fakeTenanted) GetPartitionID() string { return f.PartitionID }
func (f *fakeTenanted) GetAccessID() string    { return f.AccessID }
func (f *fakeTenanted) SetTenantID(v string)   { f.TenantID = v }
func (f *fakeTenanted) SetPartitionID(v string) { f.PartitionID = v }
func (f *fakeTenanted) SetAccessID(v string)   { f.AccessID = v }

// fakeUntenanted intentionally lacks the tenant_id/partition_id fields.
type fakeUntenanted struct {
	ID   string `gorm:"primaryKey"`
	Name string
}

// fakeUnscoped embeds the marker to opt out even though it satisfies
// the Tenanted method set.
type fakeUnscoped struct {
	fakeTenanted
	tenancy.UnscopedMarker
}

func TestEnrolledModelsPicksTenanted(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(nil, &gorm.Config{
		NamingStrategy:                           schema.NamingStrategy{},
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	// We don't actually need a real connection — tenancy.EnrolledModels
	// only uses GORM's statement parser, which works without a driver.
	_ = err

	enrolled, err := tenancy.EnrolledModels(db, []any{
		&fakeTenanted{},
		&fakeUntenanted{},
		&fakeUnscoped{},
	})
	require.NoError(t, err)
	require.Len(t, enrolled, 1, "only fakeTenanted should be enrolled")
	require.Equal(t, "tenant_id", enrolled[0].TenantColumn)
	require.Equal(t, "partition_id", enrolled[0].PartitionColumn)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tenancy/ -run TestEnrolledModels`
Expected: FAIL — `EnrolledModels` undefined.

- [ ] **Step 3: Write the implementation**

Create `tenancy/enrollment.go`:

```go
package tenancy

import (
	"fmt"

	"gorm.io/gorm"
)

// EnrolledModels filters the supplied migration models, returning
// ModelInfo for those that satisfy the Tenanted interface and do NOT
// satisfy Unscoped. Tenant and partition column names default to
// "tenant_id" / "partition_id"; future overrides can come from
// per-model tags but are not required today.
//
// The supplied *gorm.DB is used only as a statement context for table
// name resolution — no queries are executed.
func EnrolledModels(db *gorm.DB, models []any) ([]ModelInfo, error) {
	if len(models) == 0 {
		return nil, nil
	}
	enrolled := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		if m == nil {
			continue
		}
		if _, isUnscoped := m.(Unscoped); isUnscoped {
			continue
		}
		if _, isTenanted := m.(Tenanted); !isTenanted {
			continue
		}
		table, err := tableNameFor(db, m)
		if err != nil {
			return nil, err
		}
		if table == "" {
			continue
		}
		enrolled = append(enrolled, ModelInfo{
			Table:           table,
			TenantColumn:    "tenant_id",
			PartitionColumn: "partition_id",
		})
	}
	return enrolled, nil
}

// tableNameFor resolves the SQL table name GORM uses for the supplied
// model, honouring naming-strategy overrides (snake_case, plural,
// prefix, etc.) without hardcoding conventions.
func tableNameFor(db *gorm.DB, m any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", fmt.Errorf("tenancy: parse model: %w", err)
	}
	return stmt.Table, nil
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./tenancy/ -run TestEnrolledModels -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tenancy/enrollment.go tenancy/enrollment_test.go
git commit -m "feat(tenancy): structural enrollment via Tenanted / Unscoped"
```

---

## Task 10: Postgres tenancy provider — embedded SQL

**Files:**
- Create: `tenancy/postgres/sql.go`

- [ ] **Step 1: Write the SQL constants**

Create `tenancy/postgres/sql.go`:

```go
// Package postgres provides the Postgres concrete tenancy.Provider.
// It installs Row-Level Security policies at migration time and binds
// per-request tenancy state via pgxpool BeforeAcquire / AfterRelease
// hooks. Combined, application code never references tenant_id or
// partition_id directly.
package postgres

// appTenancyMatchesFn is the Postgres-side helper installed once per
// database. It reads the app.tenant_id and app.partition_id session
// variables set by the per-acquire hook and answers whether a row is
// visible to the current principal.
//
//   - tenant_id is single-valued: row matches when the row's tenant
//     equals the setting, or the setting is empty (system services /
//     migrations are unscoped — same default as the no-claims path).
//   - partition_id is a comma-separated list: row matches when the
//     row's partition appears in the list, or the setting is empty.
//     Principals that legitimately span multiple partitions thus see
//     rows from every partition they belong to without any application
//     code awareness.
const appTenancyMatchesFn = `
CREATE OR REPLACE FUNCTION app_tenancy_matches(
    row_tenant_id text,
    row_partition_id text
) RETURNS boolean AS $$
BEGIN
    RETURN (
        current_setting('app.tenant_id', true) IS NULL
        OR current_setting('app.tenant_id', true) = ''
        OR row_tenant_id = current_setting('app.tenant_id', true)
    ) AND (
        current_setting('app.partition_id', true) IS NULL
        OR current_setting('app.partition_id', true) = ''
        OR row_partition_id = ANY(string_to_array(current_setting('app.partition_id', true), ','))
    );
END;
$$ LANGUAGE plpgsql STABLE;
`

// alterEnableRLS, alterForceRLS, dropPolicy, createPolicy are the per-
// table statements applied by Install. Quoting is performed by the
// caller (via dialect.QuoteIdentifier) — these strings receive a
// pre-quoted table name and pre-quoted column names.
const (
	alterEnableRLS = "ALTER TABLE %s ENABLE ROW LEVEL SECURITY"
	alterForceRLS  = "ALTER TABLE %s FORCE ROW LEVEL SECURITY"
	dropPolicy     = "DROP POLICY IF EXISTS app_tenancy_isolation ON %s"
	createPolicy   = "CREATE POLICY app_tenancy_isolation ON %s FOR ALL " +
		"USING (app_tenancy_matches(%s, %s)) " +
		"WITH CHECK (app_tenancy_matches(%s, %s))"
)
```

- [ ] **Step 2: Verify build**

Run: `go build ./tenancy/postgres/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add tenancy/postgres/sql.go
git commit -m "feat(tenancy/postgres): embed RLS DDL fragments"
```

---

## Task 11: Postgres tenancy provider — Install + WireAdapter

**Files:**
- Create: `tenancy/postgres/provider.go`

- [ ] **Step 1: Write the provider**

Create `tenancy/postgres/provider.go`:

```go
package postgres

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/tenancy"
)

// Provider is the Postgres concrete tenancy.Provider. It installs RLS
// policies during Migrate and binds per-request tenancy state via
// pgxpool acquire/release hooks (no transactions required).
type Provider struct{}

// New returns a fresh Postgres tenancy provider.
func New() *Provider { return &Provider{} }

// Name implements tenancy.Provider.
func (*Provider) Name() string { return "postgres-rls" }

// Capabilities implements tenancy.Provider.
func (*Provider) Capabilities() tenancy.Capabilities {
	return tenancy.Capabilities{EnforcesAtStorage: true}
}

// Install implements tenancy.Provider. Idempotent:
//   - CREATE OR REPLACE for the SQL function
//   - DROP POLICY IF EXISTS / CREATE POLICY pair per table
//   - ALTER TABLE … ENABLE / FORCE ROW LEVEL SECURITY (Postgres
//     no-ops if already enabled).
func (*Provider) Install(_ context.Context, db *gorm.DB, models []tenancy.ModelInfo) error {
	if db == nil {
		return fmt.Errorf("tenancy/postgres: nil db")
	}
	if err := db.Exec(appTenancyMatchesFn).Error; err != nil {
		return fmt.Errorf("install app_tenancy_matches: %w", err)
	}
	for _, m := range models {
		if applyErr := applyTenancyPolicy(db, m); applyErr != nil {
			return fmt.Errorf("enable RLS on %s: %w", m.Table, applyErr)
		}
	}
	return nil
}

// applyTenancyPolicy emits the four idempotent statements for one table.
func applyTenancyPolicy(db *gorm.DB, m tenancy.ModelInfo) error {
	quoted := pgQuoteIdent(m.Table)
	tenantCol := pgQuoteIdent(m.TenantColumn)
	partitionCol := pgQuoteIdent(m.PartitionColumn)

	stmts := []string{
		fmt.Sprintf(alterEnableRLS, quoted),
		fmt.Sprintf(alterForceRLS, quoted),
		fmt.Sprintf(dropPolicy, quoted),
		fmt.Sprintf(createPolicy, quoted, tenantCol, partitionCol, tenantCol, partitionCol),
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// WireAdapter implements tenancy.Provider. Registers a BeforeAcquire
// hook that pushes Claims-derived session vars onto the connection,
// and an AfterRelease hook that resets them so connections are returned
// to the pool clean.
func (p *Provider) WireAdapter(adapter dialect.DialectAdapter) error {
	if adapter == nil {
		return fmt.Errorf("tenancy/postgres: nil adapter")
	}
	if err := adapter.RegisterAcquireHook(p.beforeAcquire); err != nil {
		return fmt.Errorf("register acquire hook: %w", err)
	}
	if err := adapter.RegisterReleaseHook(p.afterRelease); err != nil {
		return fmt.Errorf("register release hook: %w", err)
	}
	return nil
}

// WireGorm implements tenancy.Provider. Postgres-RLS is enforced at
// the connection-acquire level, so no per-query GORM plugin is needed.
func (*Provider) WireGorm(_ *gorm.DB) error { return nil }

// beforeAcquire pulls the tenancy.Claims from ctx and pushes them onto
// the pgx connection as session variables. is_local=false means the
// vars persist for the conn's lifetime (not just one tx); afterRelease
// resets them. If claims are empty or Skip is set, no vars are pushed
// — the RLS policy's empty-match-all branch applies.
func (*Provider) beforeAcquire(ctx context.Context, conn dialect.DialectConn) error {
	claims := tenancy.ClaimsFromContext(ctx)
	if claims == nil || claims.IsEmpty() || claims.Skip {
		return nil
	}
	if err := conn.Exec(
		ctx,
		"SELECT set_config('app.tenant_id', $1, false)",
		claims.TenantID,
	); err != nil {
		return fmt.Errorf("set app.tenant_id: %w", err)
	}
	if err := conn.Exec(
		ctx,
		"SELECT set_config('app.partition_id', $1, false)",
		strings.Join(claims.PartitionIDs, ","),
	); err != nil {
		return fmt.Errorf("set app.partition_id: %w", err)
	}
	return nil
}

// afterRelease resets the session vars so subsequent acquires that
// don't carry tenancy claims see clean defaults.
func (*Provider) afterRelease(ctx context.Context, conn dialect.DialectConn) error {
	if err := conn.Exec(ctx, "RESET app.tenant_id"); err != nil {
		return err
	}
	return conn.Exec(ctx, "RESET app.partition_id")
}

var _ tenancy.Provider = (*Provider)(nil)
```

- [ ] **Step 2: Verify build (this depends on Task 12's ClaimsFromContext)**

The provider references `tenancy.ClaimsFromContext`, which is added in Task 12. The build will fail until Task 12 lands. Skip to Task 12 to introduce the context helpers, then come back to verify.

Run (now expected to fail): `go build ./tenancy/postgres/...`
Expected: FAIL — `ClaimsFromContext` undefined.

- [ ] **Step 3: Commit**

```bash
git add tenancy/postgres/provider.go
git commit -m "feat(tenancy/postgres): Install (RLS) + WireAdapter hooks"
```

---

## Task 12: Tenancy Claims context helpers + ClaimsFromAuth

**Files:**
- Modify: `tenancy/claims.go`
- Modify: `tenancy/claims_test.go`

- [ ] **Step 1: Write failing tests**

Update the imports block of `tenancy/claims_test.go` to include:

```go
import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/v2/security"
	"github.com/pitabwire/frame/v2/tenancy"
)
```

Append the following test functions to `tenancy/claims_test.go`:

```go
func TestWithClaimsAndClaimsFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Nil(t, tenancy.ClaimsFromContext(ctx))

	c := &tenancy.Claims{TenantID: "t1", PartitionIDs: []string{"p1"}}
	ctx2 := tenancy.WithClaims(ctx, c)
	got := tenancy.ClaimsFromContext(ctx2)
	require.NotNil(t, got)
	require.Equal(t, "t1", got.TenantID)
	require.Equal(t, []string{"p1"}, got.PartitionIDs)
}

func TestClaimsFromContextFallsBackToAuth(t *testing.T) {
	t.Parallel()

	auth := &security.AuthenticationClaims{TenantID: "t1", PartitionID: "p1", AccessID: "a1"}
	ctx := auth.ClaimsToContext(context.Background())

	got := tenancy.ClaimsFromContext(ctx)
	require.NotNil(t, got)
	require.Equal(t, "t1", got.TenantID)
	require.Equal(t, []string{"p1"}, got.PartitionIDs)
	require.Equal(t, "a1", got.AccessID)
	require.False(t, got.Skip)
}

func TestClaimsFromAuthSkipForInternalSystem(t *testing.T) {
	t.Parallel()

	auth := &security.AuthenticationClaims{
		TenantID:    "t1",
		PartitionID: "p1",
		Roles:       []string{security.ConstantSystemInternalRole},
	}
	ctx := auth.ClaimsToContext(context.Background())
	got := tenancy.ClaimsFromContext(ctx)
	require.NotNil(t, got)
	require.True(t, got.Skip, "internal system caller should yield Skip=true")
}

func TestWithExtraPartitionsExtendsCurrent(t *testing.T) {
	t.Parallel()

	auth := &security.AuthenticationClaims{TenantID: "t1", PartitionID: "p1"}
	ctx := auth.ClaimsToContext(context.Background())
	ctx = tenancy.WithExtraPartitions(ctx, "p2", "p3")

	got := tenancy.ClaimsFromContext(ctx)
	require.NotNil(t, got)
	require.Equal(t, "t1", got.TenantID)
	require.Equal(t, []string{"p1", "p2", "p3"}, got.PartitionIDs)
}

func TestWithExtraPartitionsNoOpWithoutClaims(t *testing.T) {
	t.Parallel()

	ctx := tenancy.WithExtraPartitions(context.Background(), "p1")
	require.Nil(t, tenancy.ClaimsFromContext(ctx))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tenancy/ -run "TestWithClaims|TestClaimsFromContext|TestClaimsFromAuth|TestWithExtraPartitions"`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Add the helpers**

Append to `tenancy/claims.go`:

```go
import (
	"context"

	"github.com/pitabwire/frame/v2/security"
)

// claimsKey is the unexported context key under which Claims are
// stored. Using an unexported empty struct prevents collisions with
// other packages' context values.
type claimsKey struct{}

// WithClaims binds Claims to ctx. Returns the parent ctx unchanged
// when c is nil to avoid hiding a "no claims" signal behind a
// non-empty context.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFromContext returns the bound Claims with graceful fallback:
//
//  1. Explicit Claims bound via WithClaims (fastest path).
//  2. Derived from security.AuthenticationClaims if present in ctx
//     (job workers / services that haven't run the tenancy interceptor
//     still get correct enforcement).
//  3. nil — caller is unscoped (system services, migrations).
func ClaimsFromContext(ctx context.Context) *Claims {
	if v, ok := ctx.Value(claimsKey{}).(*Claims); ok {
		return v
	}
	if auth := security.ClaimsFromContext(ctx); auth != nil {
		return ClaimsFromAuth(ctx, auth)
	}
	return nil
}

// ClaimsFromAuth derives Claims from auth claims using the frame
// default mapping:
//
//	TenantID     = auth.GetTenantID()
//	PartitionIDs = auth.GetPartitionIDs()
//	AccessID     = auth.GetAccessID()
//	Skip         = auth.IsInternalSystem() || security.IsTenancyChecksOnClaimSkipped(ctx)
//
// Not overridable — callers needing different semantics build Claims
// directly and bind via WithClaims.
func ClaimsFromAuth(ctx context.Context, auth *security.AuthenticationClaims) *Claims {
	if auth == nil {
		return nil
	}
	return &Claims{
		TenantID:     auth.GetTenantID(),
		PartitionIDs: auth.GetPartitionIDs(),
		AccessID:     auth.GetAccessID(),
		Skip:         auth.IsInternalSystem() || security.IsTenancyChecksOnClaimSkipped(ctx),
	}
}

// WithExtraPartitions reads the current Claims from ctx, extends them
// with the supplied partition IDs (preserving TenantID, AccessID, Skip),
// and binds the extended Claims to a child ctx. Returns ctx unchanged
// when no claims are present.
//
// Use for service-on-behalf-of flows, cross-branch reporting, or any
// case where a principal legitimately needs visibility over additional
// partitions without changing tenant.
func WithExtraPartitions(ctx context.Context, partitionIDs ...string) context.Context {
	current := ClaimsFromContext(ctx)
	if current == nil {
		return ctx
	}
	extended := current.ExtendPartitions(partitionIDs...)
	return WithClaims(ctx, extended)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./tenancy/ -v`
Expected: PASS — all claims tests including the new ones.

Run: `go build ./tenancy/postgres/...`
Expected: success — the Postgres provider now finds `ClaimsFromContext`.

- [ ] **Step 5: Commit**

```bash
git add tenancy/claims.go tenancy/claims_test.go
git commit -m "feat(tenancy): claims context binding, auth derivation, extension helper"
```

---

## Task 13: Tenancy interceptor (Connect)

**Files:**
- Create: `tenancy/interceptor.go`

- [ ] **Step 1: Write the interceptor**

Create `tenancy/interceptor.go`:

```go
package tenancy

import (
	"context"

	"connectrpc.com/connect"

	"github.com/pitabwire/frame/v2/security"
)

// NewClaimsInterceptor returns a Connect interceptor that derives
// tenancy.Claims from auth claims and binds them to ctx for downstream
// code. The interceptor performs no database activity — it is cheap
// and safe for streaming RPCs.
//
// Register after the authentication interceptor so auth claims are
// available when this interceptor reads them.
func NewClaimsInterceptor() connect.Interceptor {
	return &claimsInterceptor{}
}

type claimsInterceptor struct{}

func (*claimsInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return next(bindClaims(ctx), req)
	}
}

func (*claimsInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (*claimsInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(bindClaims(ctx), conn)
	}
}

// bindClaims derives Claims from auth claims (if present) and binds
// them. If no auth claims are in ctx, returns ctx unchanged.
func bindClaims(ctx context.Context) context.Context {
	auth := security.ClaimsFromContext(ctx)
	if auth == nil {
		return ctx
	}
	return WithClaims(ctx, ClaimsFromAuth(ctx, auth))
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./tenancy/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add tenancy/interceptor.go
git commit -m "feat(tenancy): Connect claims interceptor (no transactions)"
```

---

## Task 14: Postgres tenancy provider integration tests

**Files:**
- Create: `tenancy/postgres/provider_test.go`

- [ ] **Step 1: Write the integration tests**

Create `tenancy/postgres/provider_test.go`:

```go
package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/data"
	"github.com/pitabwire/frame/v2/datastore/dialect"
	dialectpg "github.com/pitabwire/frame/v2/datastore/dialect/postgres"
	"github.com/pitabwire/frame/v2/frametests/definition"
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"
	"github.com/pitabwire/frame/v2/tests"
)

type ProviderTestSuite struct {
	tests.BaseTestSuite
}

func TestProviderSuite(t *testing.T) {
	suite.Run(t, &ProviderTestSuite{})
}

// rlsEntity is a test model embedding BaseModel so it satisfies
// tenancy.Tenanted and gets RLS installed.
type rlsEntity struct {
	data.BaseModel
	Name string `gorm:"type:varchar(64)"`
}

func (rlsEntity) TableName() string { return "rls_entities" }

func (s *ProviderTestSuite) wireProviderWithPool(t *testing.T, ctx context.Context, dsn string) (*gorm.DB, dialect.DialectAdapter, *tenpg.Provider) {
	t.Helper()
	adapter := dialectpg.New()
	prov := tenpg.New()
	require.NoError(t, prov.WireAdapter(adapter))

	dialector, sqlDB, err := adapter.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 4})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	return db, adapter, prov
}

func (s *ProviderTestSuite) TestInstallIdempotent() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		db, _, prov := s.wireProviderWithPool(t, ctx, dsn)
		require.NoError(t, db.AutoMigrate(&rlsEntity{}))

		models := []tenancy.ModelInfo{{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"}}
		require.NoError(t, prov.Install(ctx, db, models), "first install")
		require.NoError(t, prov.Install(ctx, db, models), "second install (idempotent)")

		// Verify the policy exists exactly once.
		var count int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM pg_policies WHERE tablename = ? AND policyname = ?",
			"rls_entities", "app_tenancy_isolation",
		).Scan(&count).Error)
		require.Equal(t, int64(1), count)
	})
}

func (s *ProviderTestSuite) TestRLSFiltersAcrossTenants() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		db, _, prov := s.wireProviderWithPool(t, ctx, dsn)
		require.NoError(t, db.AutoMigrate(&rlsEntity{}))
		require.NoError(t, prov.Install(ctx, db, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))

		// Seed across two tenants without claims so RLS doesn't block.
		require.NoError(t, db.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
			Name:      "row-T1",
		}).Error)
		require.NoError(t, db.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
			Name:      "row-T2",
		}).Error)

		// Bind T1 claims and query — only T1's row should be visible.
		ctxT1 := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1"},
		})
		var got []rlsEntity
		require.NoError(t, db.WithContext(ctxT1).Find(&got).Error)
		require.Len(t, got, 1)
		require.Equal(t, "row-T1", got[0].Name)

		// Switch to T2 — only T2's row.
		ctxT2 := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T2",
			PartitionIDs: []string{"P2"},
		})
		got = nil
		require.NoError(t, db.WithContext(ctxT2).Find(&got).Error)
		require.Len(t, got, 1)
		require.Equal(t, "row-T2", got[0].Name)
	})
}

func (s *ProviderTestSuite) TestRLSMultiPartitionPrincipal() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		db, _, prov := s.wireProviderWithPool(t, ctx, dsn)
		require.NoError(t, db.AutoMigrate(&rlsEntity{}))
		require.NoError(t, prov.Install(ctx, db, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))

		// Seed three rows for tenant T1 across three partitions.
		for _, p := range []string{"P1", "P2", "P3"} {
			require.NoError(t, db.Create(&rlsEntity{
				BaseModel: data.BaseModel{TenantID: "T1", PartitionID: p},
				Name:      "row-" + p,
			}).Error)
		}

		// Principal with access to P1 + P3 should see exactly those two.
		ctxMulti := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1", "P3"},
		})
		var got []rlsEntity
		require.NoError(t, db.WithContext(ctxMulti).Order("name").Find(&got).Error)
		require.Len(t, got, 2)
		require.Equal(t, "row-P1", got[0].Name)
		require.Equal(t, "row-P3", got[1].Name)
	})
}

func (s *ProviderTestSuite) TestSkipClaimsBypassEnforcement() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		db, _, prov := s.wireProviderWithPool(t, ctx, dsn)
		require.NoError(t, db.AutoMigrate(&rlsEntity{}))
		require.NoError(t, prov.Install(ctx, db, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))

		require.NoError(t, db.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
			Name:      "row-T1",
		}).Error)
		require.NoError(t, db.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
			Name:      "row-T2",
		}).Error)

		// Skip=true should make every row visible.
		ctxSkip := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "anything",
			PartitionIDs: []string{"anything"},
			Skip:         true,
		})
		var got []rlsEntity
		require.NoError(t, db.WithContext(ctxSkip).Find(&got).Error)
		require.Len(t, got, 2)
	})
}

func (s *ProviderTestSuite) TestAfterReleaseResetsSessionState() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		db, _, prov := s.wireProviderWithPool(t, ctx, dsn)
		require.NoError(t, db.AutoMigrate(&rlsEntity{}))
		require.NoError(t, prov.Install(ctx, db, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))

		// Bind claims and issue a no-op query so the hook fires.
		ctxScoped := tenancy.WithClaims(ctx, &tenancy.Claims{TenantID: "T1", PartitionIDs: []string{"P1"}})
		require.NoError(t, db.WithContext(ctxScoped).Exec("SELECT 1").Error)

		// Subsequent acquire with no claims must see empty session vars.
		var got string
		require.NoError(t, db.Raw(`SELECT current_setting('app.tenant_id', true)`).Scan(&got).Error)
		require.Empty(t, got, "session state must be reset by AfterRelease")
	})
}
```

- [ ] **Step 2: Run integration tests**

Run: `go test ./tenancy/postgres/... -v`
Expected: PASS — all five subtests.

- [ ] **Step 3: Commit**

```bash
git add tenancy/postgres/provider_test.go
git commit -m "test(tenancy/postgres): RLS install + per-acquire enforcement (testcontainers)"
```

---

## Task 15: Pool options — adapter + provider knobs

**Files:**
- Modify: `datastore/pool/options.go`

- [ ] **Step 1: Add the options**

Open `datastore/pool/options.go` and add at the end of the file:

```go
import (
	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/tenancy"
)

// WithDialectAdapter sets the database driver adapter for this pool.
// When omitted, the pool uses the Postgres adapter.
func WithDialectAdapter(adapter dialect.DialectAdapter) Option {
	return func(o *Options) {
		o.DialectAdapter = adapter
	}
}

// WithTenancyProvider sets the tenancy provider for this pool. When
// omitted, the pool uses the Postgres-RLS provider. Pass nil to
// disable tenancy enforcement (useful in unit tests that want raw
// database access).
func WithTenancyProvider(prov tenancy.Provider) Option {
	return func(o *Options) {
		o.TenancyProvider = prov
		o.TenancyProviderSet = true
	}
}
```

Extend the `Options` struct in the same file:

```go
type Options struct {
	Connections []Connection

	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration

	PreferSimpleProtocol   bool
	SkipDefaultTransaction bool

	TraceConfig     config.ConfigurationDatabaseTracing
	InsertBatchSize int

	PreparedStatements bool

	// DialectAdapter and TenancyProvider are resolved by NewPool with
	// Postgres defaults when unset. TenancyProviderSet distinguishes
	// "use default" from "explicitly disabled (nil)".
	DialectAdapter     dialect.DialectAdapter
	TenancyProvider    tenancy.Provider
	TenancyProviderSet bool
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./datastore/pool/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add datastore/pool/options.go
git commit -m "feat(pool): WithDialectAdapter + WithTenancyProvider options"
```

---

## Task 16: Pool implementation — adapter + provider plumbing, drop tenancy methods

**Files:**
- Modify: `datastore/pool/interface.go`
- Modify: `datastore/pool/implementation.go`
- Modify: `datastore/pool/connection.go`
- Delete: `datastore/pool/rls.go`

This is the largest single task. It removes the tx-in-context API surface, swaps the Postgres driver for the adapter, and wires the tenancy provider's Install + WireAdapter calls. Tests in `datastore/repository_test.go` continue to pass — they exercise `pool.DB(ctx, _)` and `pool.Migrate(...)`, both of which stay.

- [ ] **Step 1: Rewrite `datastore/pool/interface.go`**

Replace the entire file contents:

```go
package pool

import (
	"context"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/migration"
)

// Connection represents a single database connection configuration.
type Connection struct {
	DSN      string
	ReadOnly bool
}

// Pool is the minimal connection-pool surface. Tenancy enforcement is
// applied transparently by the dialect adapter + tenancy provider
// composed via pool options; the Pool itself exposes only routing,
// migration, and lifecycle methods. Multi-statement atomicity is
// caller-driven via gorm's db.Transaction(fn).
type Pool interface {
	// DB returns a *gorm.DB routed to a writable (readOnly=false) or
	// read-only (readOnly=true) connection. The returned session has
	// tenancy applied at the connection level — callers do not need
	// to filter by tenant_id or partition_id explicitly.
	DB(ctx context.Context, readOnly bool) *gorm.DB

	// AddConnection opens a new physical connection and adds it to the
	// pool. May be called multiple times for read/write replication.
	AddConnection(ctx context.Context, opts ...Option) error

	// CanMigrate reports whether this pool was constructed in a mode
	// that permits running migrations.
	CanMigrate() bool

	// SaveMigration records the supplied patches in the migrations
	// metadata table without applying them.
	SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error

	// Migrate finds missing migrations, applies them, and installs
	// tenancy enforcement (via the configured tenancy.Provider) on
	// every enrolled model.
	Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error

	// Close gracefully shuts down all opened connections.
	Close(ctx context.Context)
}
```

- [ ] **Step 2: Rewrite `datastore/pool/implementation.go`**

Replace with:

```go
package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pitabwire/util"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
	dialectpg "github.com/pitabwire/frame/v2/datastore/dialect/postgres"
	"github.com/pitabwire/frame/v2/datastore/migration"
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"
)

const migrationAdvisoryLockID int64 = 82548391244719
const migrationLockRetryInterval = 200 * time.Millisecond

type pool struct {
	readIdx     uint64
	writeIdx    uint64
	mu          sync.RWMutex
	allReadDBs  []*gorm.DB
	allWriteDBs []*gorm.DB

	shouldDoMigrations bool

	adapter  dialect.DialectAdapter
	provider tenancy.Provider // may be nil if WithTenancyProvider(nil) was used
}

// NewPool constructs a pool. Options may set the dialect adapter and
// tenancy provider; defaults are Postgres + Postgres-RLS. The provider
// is wired into the adapter immediately, so hooks attach to every
// subsequently-opened connection.
func NewPool(_ context.Context, opts ...Option) Pool {
	o := &Options{
		PreferSimpleProtocol:   true,
		SkipDefaultTransaction: true,
		InsertBatchSize:        1000, //nolint:mnd // default insert batch size
		PreparedStatements:     true,
	}
	for _, opt := range opts {
		opt(o)
	}

	adapter := o.DialectAdapter
	if adapter == nil {
		adapter = dialectpg.New()
	}

	var provider tenancy.Provider
	if o.TenancyProviderSet {
		provider = o.TenancyProvider
	} else {
		provider = tenpg.New()
	}

	if provider != nil {
		if err := provider.WireAdapter(adapter); err != nil {
			// Cannot return error from NewPool without changing many
			// callers; fall back to a panic that is loud at boot.
			panic("pool: tenancy provider WireAdapter: " + err.Error())
		}
	}

	return &pool{
		allReadDBs:         []*gorm.DB{},
		allWriteDBs:        []*gorm.DB{},
		shouldDoMigrations: true,
		mu:                 sync.RWMutex{},
		adapter:            adapter,
		provider:           provider,
	}
}

// AddConnection opens a new physical connection through the adapter.
func (s *pool) AddConnection(ctx context.Context, opts ...Option) error {
	o := &Options{
		PreferSimpleProtocol:   true,
		SkipDefaultTransaction: true,
		InsertBatchSize:        1000, //nolint:mnd
		PreparedStatements:     true,
	}
	for _, opt := range opts {
		opt(o)
	}

	for _, conn := range o.Connections {
		db, err := s.createConnection(ctx, conn.DSN, o)
		if err != nil {
			return err
		}
		if s.provider != nil {
			if wireErr := s.provider.WireGorm(db); wireErr != nil {
				return wireErr
			}
		}

		s.mu.Lock()
		if conn.ReadOnly {
			s.allReadDBs = append(s.allReadDBs, db)
		} else {
			s.allWriteDBs = append(s.allWriteDBs, db)
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *pool) Close(_ context.Context) {
	s.mu.RLock()
	readDBs := append([]*gorm.DB(nil), s.allReadDBs...)
	writeDBs := append([]*gorm.DB(nil), s.allWriteDBs...)
	s.mu.RUnlock()

	for _, db := range append(readDBs, writeDBs...) {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

// DB returns a tenancy-aware *gorm.DB. Tenancy is enforced at the
// connection-acquire level by the provider's hook — DB itself does
// not filter by tenant_id / partition_id.
func (s *pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var idx *uint64
	var selectedDB *gorm.DB
	if readOnly {
		idx = &s.readIdx
		if len(s.allReadDBs) != 0 {
			selectedDB = s.selectOne(s.allReadDBs, idx)
		}
	}
	if selectedDB == nil {
		idx = &s.writeIdx
		selectedDB = s.selectOne(s.allWriteDBs, idx)
	}
	if selectedDB == nil {
		return nil
	}
	return selectedDB.Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).
		WithContext(ctx)
}

func (s *pool) selectOne(p []*gorm.DB, idx *uint64) *gorm.DB {
	if len(p) == 0 {
		return nil
	}
	pos := atomic.AddUint64(idx, 1)
	i := (pos - 1) % uint64(len(p))
	return p[i]
}

func (s *pool) CanMigrate() bool { return s.shouldDoMigrations }

func (s *pool) SaveMigration(ctx context.Context, migrationPatches ...*migration.Patch) error {
	executor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB { return s.DB(ctx, false) })
	for _, p := range migrationPatches {
		if err := executor.SaveMigrationString(ctx, p.Name, p.Patch, p.RevertPatch); err != nil {
			return err
		}
	}
	return nil
}

// Migrate runs AutoMigrate on the supplied models, invokes
// Provider.Install on the tenancy-enrolled subset, and applies any
// patch-based migrations from migrationsDirPath. Uses the adapter's
// advisory lock to serialise concurrent boots.
func (s *pool) Migrate(ctx context.Context, migrationsDirPath string, migrations ...any) error {
	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	db := s.DB(ctx, false)
	if db == nil {
		return errors.New("migrate datastore: no writable database configured")
	}

	migrator := db.Migrator()

	unlock, lockErr := s.adapter.AdvisoryLock(ctx, db, migrationAdvisoryLockID)
	if lockErr != nil {
		util.Log(ctx).WithError(lockErr).
			Warn("MigrateDatastore -- couldn't acquire advisory lock, continuing without lock")
	}
	if unlock != nil {
		defer unlock()
	}

	if err := s.ensureMigrationTable(migrator); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't create migration table")
		return err
	}

	if len(migrations) > 0 {
		if err := migrator.AutoMigrate(migrations...); err != nil {
			util.Log(ctx).WithError(err).Error("MigrateDatastore -- couldn't auto migrate")
			return err
		}
		if s.provider != nil {
			enrolled, err := tenancy.EnrolledModels(db, migrations)
			if err != nil {
				return err
			}
			if err := s.provider.Install(ctx, db, enrolled); err != nil {
				util.Log(ctx).WithError(err).Error("MigrateDatastore -- tenancy install failed")
				return err
			}
		}
	}

	executor := migration.NewMigrator(ctx, func(ctx context.Context) *gorm.DB { return s.DB(ctx, false) })
	if err := executor.ScanMigrationFiles(ctx, migrationsDirPath); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- Error scanning for new migrations")
		return err
	}
	if err := executor.ApplyNewMigrations(ctx); err != nil {
		util.Log(ctx).WithError(err).Error("MigrateDatastore -- Error applying migrations ")
		return err
	}
	return nil
}

func (s *pool) ensureMigrationTable(migrator gorm.Migrator) error {
	if migrator.HasTable(&migration.Migration{}) {
		return nil
	}
	err := migrator.CreateTable(&migration.Migration{})
	if err != nil && !s.adapter.IsRelationAlreadyExistsErr(err) {
		return err
	}
	return nil
}
```

Note: `migrationLockRetryInterval` is now unused in this file (the adapter owns it). Remove it.

- [ ] **Step 3: Rewrite `datastore/pool/connection.go`**

Replace with a thin file that delegates to the adapter:

```go
package pool

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/v2/datastore/dialect"
)

// createConnection asks the adapter to open a *gorm.DB. The adapter
// owns all driver-specific concerns (DSN parsing, pgxpool tuning,
// otel wiring, hook attachment).
func (s *pool) createConnection(ctx context.Context, dsn string, poolOpts *Options) (*gorm.DB, error) {
	cleanDSN, err := s.adapter.NormalizeDSN(dsn)
	if err != nil {
		return nil, err
	}

	dialector, _, err := s.adapter.OpenConnection(ctx, cleanDSN, dialect.ConnectionOptions{
		MaxOpen:                poolOpts.MaxOpen,
		MaxLifetime:            poolOpts.MaxLifetime,
		PreferSimpleProtocol:   poolOpts.PreferSimpleProtocol,
		SkipDefaultTransaction: poolOpts.SkipDefaultTransaction,
		InsertBatchSize:        poolOpts.InsertBatchSize,
		PreparedStatements:     poolOpts.PreparedStatements,
		Logger:                 datastoreLogger(ctx, poolOpts.TraceConfig),
	})
	if err != nil {
		return nil, err
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 datastoreLogger(ctx, poolOpts.TraceConfig),
		SkipDefaultTransaction: poolOpts.SkipDefaultTransaction,
		PrepareStmt:            poolOpts.PreparedStatements,
		CreateBatchSize:        poolOpts.InsertBatchSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open GORM connection: %w", err)
	}
	return gormDB, nil
}
```

- [ ] **Step 4: Delete `datastore/pool/rls.go`**

```bash
git rm datastore/pool/rls.go
```

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: build fails inside `security/interceptors/connect/tenancy_tx.go` because it references `pool.WithRequestTx`, which no longer exists.

This is expected — Task 17 deletes that file.

- [ ] **Step 6: Commit (intermediate breakage acceptable; resolved in Task 17)**

```bash
git add datastore/pool/interface.go datastore/pool/implementation.go datastore/pool/connection.go
git rm datastore/pool/rls.go
git commit -m "refactor(pool): compose dialect.DialectAdapter + tenancy.Provider; drop tx-in-context"
```

---

## Task 17: Delete the old tenancy-tx interceptor

**Files:**
- Delete: `security/interceptors/connect/tenancy_tx.go`

- [ ] **Step 1: Delete the file**

```bash
git rm security/interceptors/connect/tenancy_tx.go
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: success — pool no longer has `WithRequestTx`, the only file that referenced it is gone.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor(security): remove tenancy-tx interceptor; superseded by tenancy.NewClaimsInterceptor"
```

---

## Task 18: Delete `datastore/scopes` package

**Files:**
- Delete: `datastore/scopes/tenancy.go`
- Delete: `datastore/scopes/` (directory)

- [ ] **Step 1: Confirm no remaining references**

Run: `grep -rn "datastore/scopes\|scopes.TenancyPartition" /home/j/code/pitabwire/frame --include="*.go"`
Expected: no matches (or only inside the file about to be deleted).

If references remain, fix them before deleting. None should exist after Task 16.

- [ ] **Step 2: Delete the file and directory**

```bash
git rm datastore/scopes/tenancy.go
rmdir datastore/scopes/
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(datastore): delete scopes package (redundant with RLS)"
```

---

## Task 19: Service wiring — default provider + accessor

**Files:**
- Modify: `service.go`
- Modify: `options_datastore.go`

- [ ] **Step 1: Add the field + accessor on Service**

Inspect `service.go` and locate the `Service` struct definition. Add a `tenancyProvider` field next to `datastoreManager`. The exact location depends on the current struct; the pattern matches the existing `securityManager` field.

Example diff:

```go
type Service struct {
    // ... existing fields ...
    datastoreManager datastore.Manager
    tenancyProvider  tenancy.Provider   // NEW
    // ... existing fields ...
}
```

Then add an accessor near the existing `DatastoreManager()` method:

```go
// TenancyProvider returns the tenancy provider wired for the default
// pool. Used by tests and diagnostics. May be nil when tenancy has
// been explicitly disabled via WithTenancyProvider(nil).
func (s *Service) TenancyProvider() tenancy.Provider {
    return s.tenancyProvider
}
```

Add the import `"github.com/pitabwire/frame/v2/tenancy"` to the import block.

- [ ] **Step 2: Wire defaults in options_datastore.go**

Open `options_datastore.go`. After the existing `WithDatastore` function, add:

```go
import (
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"
)

// WithTenancyProvider overrides the default Postgres-RLS tenancy
// provider. Pass nil to disable tenancy enforcement entirely.
//
// Must be combined with WithDatastore; the provider is associated with
// the default pool.
func WithTenancyProvider(prov tenancy.Provider) Option {
	return func(ctx context.Context, s *Service) {
		s.tenancyProvider = prov
		// Forward to the pool's options as well so AddConnection wires
		// the same provider when constructing connections.
		_ = ctx // referenced to satisfy linter; pool wiring happens in WithDatastore
	}
}
```

Update `WithDatastore` to:

1. Default `s.tenancyProvider` to `tenpg.New()` if not already set.
2. Forward the provider through `pool.WithTenancyProvider` when constructing the default pool.

Concretely, before the existing call site that runs `WithDatastoreConnectionWithOptions(...)`, add:

```go
if s.tenancyProvider == nil {
    s.tenancyProvider = tenpg.New()
}
enrichedOpts = append(enrichedOpts, pool.WithTenancyProvider(s.tenancyProvider))
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Run existing repository tests**

Run: `go test ./datastore/... -v -count=1`
Expected: PASS — the existing `repository_test.go` continues to pass because the default Postgres-RLS provider matches the previous behaviour for one-shot calls.

- [ ] **Step 5: Commit**

```bash
git add service.go options_datastore.go
git commit -m "feat(frame): WithTenancyProvider option + svc.TenancyProvider() accessor"
```

---

## Task 20: End-to-end interceptor + repository integration test

**Files:**
- Create: `tenancy/interceptor_test.go`

- [ ] **Step 1: Write the integration test**

Create `tenancy/interceptor_test.go`:

```go
package tenancy_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/data"
	"github.com/pitabwire/frame/v2/datastore"
	"github.com/pitabwire/frame/v2/datastore/pool"
	"github.com/pitabwire/frame/v2/frametests"
	"github.com/pitabwire/frame/v2/frametests/definition"
	"github.com/pitabwire/frame/v2/security"
	"github.com/pitabwire/frame/v2/tenancy"
	"github.com/pitabwire/frame/v2/tests"
)

type interceptorEntity struct {
	data.BaseModel
	Name string
}

func (interceptorEntity) TableName() string { return "interceptor_entities" }

type InterceptorTestSuite struct {
	tests.BaseTestSuite
}

func TestInterceptorSuite(t *testing.T) {
	suite.Run(t, &InterceptorTestSuite{})
}

func (s *InterceptorTestSuite) TestClaimsInterceptorBindsAndEnforces() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		ctx, svc := frame.NewService(
			frame.WithName("interceptor-test"),
			frametests.WithNoopDriver(),
			frame.WithDatastore(pool.WithConnection(dsn, false)),
		)
		svc.Init(ctx)
		defer svc.Stop(ctx)

		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		require.NoError(t, dbPool.Migrate(ctx, "", &interceptorEntity{}))

		// Seed across tenants without claims.
		require.NoError(t, dbPool.DB(ctx, false).Create(&interceptorEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
			Name:      "row-T1",
		}).Error)
		require.NoError(t, dbPool.DB(ctx, false).Create(&interceptorEntity{
			BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
			Name:      "row-T2",
		}).Error)

		// Simulate the auth interceptor: attach AuthenticationClaims.
		auth := &security.AuthenticationClaims{TenantID: "T1", PartitionID: "P1"}
		ctxAuth := auth.ClaimsToContext(ctx)

		// Run the tenancy interceptor over a no-op handler that issues
		// a query, asserting the bound claims filter rows.
		interceptor := tenancy.NewClaimsInterceptor()
		wrapped := interceptor.WrapUnary(func(innerCtx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			var rows []interceptorEntity
			if err := dbPool.DB(innerCtx, true).Find(&rows).Error; err != nil {
				return nil, err
			}
			require.Len(t, rows, 1)
			require.Equal(t, "row-T1", rows[0].Name)
			return nil, nil
		})

		_, err := wrapped(ctxAuth, nil)
		require.NoError(t, err)
	})
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./tenancy/ -run TestInterceptorSuite -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add tenancy/interceptor_test.go
git commit -m "test(tenancy): end-to-end interceptor + repository RLS enforcement"
```

---

## Task 21: Update documentation

**Files:**
- Modify: `docs/datastore.md`

- [ ] **Step 1: Rewrite the Tenancy section**

Open `docs/datastore.md` and replace the existing "Tenancy" section (around line 61) with:

```markdown
## Tenancy

Tenancy enforcement lives in the top-level `tenancy/` package. The
default Postgres provider installs Row-Level Security policies on every
model that embeds `data.BaseModel`, and binds per-request tenancy
state to each database connection through pgxpool acquire/release
hooks.

### Wiring

```go
_, svc := frame.NewService(
    frame.WithDatastore(), // installs default Postgres adapter + RLS provider
)

// Register the lightweight claims interceptor on Connect handlers
// after your authentication interceptor:
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
```

- [ ] **Step 2: Commit**

```bash
git add docs/datastore.md
git commit -m "docs(datastore): document tenancy package and one-shot model"
```

---

## Task 22: Final lint + race-tested CI gate

- [ ] **Step 1: Run linters**

Run: `golangci-lint run ./...`
Expected: success. If new diagnostics surface (unused imports after deletions, etc.), fix them in a follow-up commit.

- [ ] **Step 2: Race-tested test suite**

Run: `go test -race ./...`
Expected: PASS for every package.

- [ ] **Step 3: If any cleanup commits were needed**

```bash
git add -A
git commit -m "chore(lint): post-refactor cleanup"
```

---

## Acceptance criteria (from the spec)

Verify each before declaring done:

- [ ] All packages listed in "Files added" exist with full unit + integration tests.
- [ ] `datastore/scopes/` is deleted.
- [ ] `Pool` interface has no tenancy methods.
- [ ] `tenancy.NewClaimsInterceptor()` replaces `connect_interceptors.NewTenancyTxInterceptor`.
- [ ] `data.BaseModel` implements `tenancy.Tenanted`.
- [ ] All existing tests in `datastore/repository_test.go` continue to pass.
- [ ] `go test -race ./...` is green.
- [ ] `golangci-lint run` is green.
- [ ] RLS enforcement verified end-to-end in `tenancy/postgres/provider_test.go` against real Postgres.
- [ ] The Postgres advisory lock for migrations preserves the existing ID (82548391244719) and retry behaviour.
