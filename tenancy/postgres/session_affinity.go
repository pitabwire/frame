package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pitabwire/util"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/v3/tenancy"
)

// ErrSessionAffinityRequired reports that the database connection does not
// keep session state between statements. This is what PgBouncer (or
// Supavisor, Neon's pooler, RDS Proxy pinning off) does in transaction or
// statement pool mode.
//
// The Postgres tenancy provider binds app.tenant_id / app.partition_id with
// session-level set_config(..., false) when a connection is acquired. Behind
// a transaction-mode pooler those settings land on whichever backend served
// that one statement. Later statements can run on a backend bound to another
// tenant (cross-tenant reads) or on one with empty settings, which
// app_tenancy_matches treats as match-all. RLS tenancy therefore requires
// direct connections or PgBouncer pool_mode=session.
var ErrSessionAffinityRequired = errors.New(
	"tenancy/postgres: connection does not keep session state (transaction/statement pooler?); " +
		"RLS tenancy requires direct Postgres or PgBouncer pool_mode=session")

// ErrSessionAffinityUnverified reports that the probe could not run to a
// verdict, typically because the pool had no free connection for the
// helper probes.
var ErrSessionAffinityUnverified = errors.New("tenancy/postgres: session affinity could not be verified")

const (
	sessionAffinityProbeTimeout = 10 * time.Second
	probeAcquireTimeout         = time.Second
	sessionAffinityProbeService = "frame-session-affinity-probe"
	probeTenantA                = "__frame_affinity_probe_a__"
	probeTenantB                = "__frame_affinity_probe_b__"
	// probeRounds and probeHelpers bound the work: at most
	// probeRounds*(1+probeHelpers) short-lived connections per check.
	probeRounds  = 3
	probeHelpers = 4
)

// CheckSessionAffinity proves, against the live database, that session
// state set on a client connection is still visible to that connection's
// later statements. It returns ErrSessionAffinityRequired when it is not.
//
// Each round:
//  1. connection A is acquired, so the tenancy acquire hook binds a sentinel
//     tenant, and A records pg_backend_pid() and app.tenant_id;
//  2. up to probeHelpers other connections each open a transaction, which
//     pins one backend apiece, and report their pid. Under session affinity
//     A owns its backend, so no helper can get it. Under a transaction
//     pooler A's backend is free between statements, so a helper getting it
//     proves the pooling;
//  3. A reads pid and app.tenant_id again. Any change means its session
//     state did not survive.
//
// A transaction pooler is detected as soon as any round observes one of
// these signals. Detection is very likely in a single call, not guaranteed,
// because the pooler picks backends. Readiness re-runs the check
// continuously, so a pooler cannot stay hidden. Direct Postgres and
// PgBouncer session mode never produce a false positive. The probe uses
// sentinel tenants that own no rows, so it is safe in every SecurityMode.
// db must be the application pool, so the real acquire hooks run.
func CheckSessionAffinity(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("tenancy/postgres: session affinity: nil db")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("tenancy/postgres: session affinity: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, sessionAffinityProbeTimeout)
	defer cancel()

	verified := false
	for range probeRounds {
		conclusive, roundErr := probeRound(ctx, sqlDB)
		if roundErr != nil {
			return roundErr
		}
		verified = verified || conclusive
	}
	if !verified {
		return fmt.Errorf("%w: no free connection for the helper probes", ErrSessionAffinityUnverified)
	}
	return nil
}

// probeRound runs one round. conclusive is false when no helper connection
// could be acquired.
func probeRound(ctx context.Context, sqlDB *sql.DB) (bool, error) {
	ctxA := probeContext(ctx, probeTenantA)
	connA, err := sqlDB.Conn(ctxA)
	if err != nil {
		return false, fmt.Errorf("tenancy/postgres: session affinity: acquire probe conn: %w", err)
	}
	defer connA.Close()

	pidBefore, settingBefore, err := probeState(ctxA, connA)
	if err != nil {
		return false, err
	}
	if settingBefore != probeTenantA {
		return false, fmt.Errorf("%w: tenant bound at acquire was not visible to the next statement (%q)",
			ErrSessionAffinityRequired, settingBefore)
	}

	helpers, release := openHelpers(ctx, sqlDB)
	defer release()
	for _, pid := range helpers {
		if pid == pidBefore {
			return false, fmt.Errorf("%w: another client was given backend %d while it held our session",
				ErrSessionAffinityRequired, pid)
		}
	}

	pidAfter, settingAfter, err := probeState(ctxA, connA)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// A blocked waiting for a backend only the helpers' open
			// transactions hold: clients are multiplexed onto backends.
			return false, fmt.Errorf("%w: probe connection blocked behind other clients' transactions",
				ErrSessionAffinityRequired)
		}
		return false, err
	}
	if pidAfter != pidBefore || settingAfter != probeTenantA {
		return false, fmt.Errorf("%w: backend pid %d -> %d, app.tenant_id %q -> %q",
			ErrSessionAffinityRequired, pidBefore, pidAfter, probeTenantA, settingAfter)
	}
	return len(helpers) > 0, nil
}

// openHelpers opens up to probeHelpers connections, each inside an open
// transaction, and returns their backend pids. Acquisition stops at the
// first connection the pool cannot supply within probeAcquireTimeout.
func openHelpers(ctx context.Context, sqlDB *sql.DB) ([]int64, func()) {
	ctxB := probeContext(ctx, probeTenantB)
	var conns []*sql.Conn
	var pids []int64
	release := func() {
		for _, c := range conns {
			_, _ = c.ExecContext(context.WithoutCancel(ctxB), "ROLLBACK")
			_ = c.Close()
		}
	}
	for range probeHelpers {
		acquireCtx, cancel := context.WithTimeout(ctxB, probeAcquireTimeout)
		conn, err := sqlDB.Conn(acquireCtx)
		cancel()
		if err != nil {
			break
		}
		conns = append(conns, conn)
		if _, err = conn.ExecContext(ctxB, "BEGIN"); err != nil {
			break
		}
		pid, _, err := probeState(ctxB, conn)
		if err != nil {
			break
		}
		pids = append(pids, pid)
	}
	return pids, release
}

func probeContext(ctx context.Context, tenantID string) context.Context {
	return tenancy.WithSystemPrincipal(ctx, tenancy.SystemPrincipal{
		ServiceName: sessionAffinityProbeService,
		Reason:      "session affinity probe",
		TenantID:    tenantID,
	})
}

func probeState(ctx context.Context, conn *sql.Conn) (int64, string, error) {
	var pid int64
	var setting sql.NullString
	err := conn.QueryRowContext(ctx,
		"SELECT pg_backend_pid(), current_setting('app.tenant_id', true)").Scan(&pid, &setting)
	if err != nil {
		return 0, "", fmt.Errorf("tenancy/postgres: session affinity probe: %w", err)
	}
	return pid, setting.String, nil
}

// SessionAffinityCheck is a readiness check that fails closed when the app
// pool does not keep session state between statements (see
// CheckSessionAffinity). Frame registers it with the arming check whenever
// the tenancy SecurityMode is Hybrid or FailClosed.
type SessionAffinityCheck struct {
	// DB returns a session for the application pool. Called on each check.
	DB func(ctx context.Context) *gorm.DB
	// NameOverride optional checker name (default tenancy_session_affinity).
	NameOverride string

	mu       sync.Mutex
	verified bool // a previous check proved session affinity
}

// Name implements frame.NamedChecker.
func (c *SessionAffinityCheck) Name() string {
	if c.NameOverride != "" {
		return c.NameOverride
	}
	return "tenancy_session_affinity"
}

// CheckHealth reports an error unless session affinity is proven.
func (c *SessionAffinityCheck) CheckHealth() error {
	if c == nil || c.DB == nil {
		return errors.New("tenancy session affinity: no database accessor")
	}
	ctx := context.Background()
	db := c.DB(ctx)
	if db == nil {
		return errors.New("tenancy session affinity: no database session")
	}
	err := CheckSessionAffinity(ctx, db)

	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case err == nil:
		c.verified = true
	case errors.Is(err, ErrSessionAffinityRequired):
		c.verified = false
		util.Log(ctx).WithError(err).
			Error("tenancy: RLS session binding is unsafe on this connection; refusing readiness")
	case errors.Is(err, ErrSessionAffinityUnverified) && c.verified:
		// Pool busy: keep the last conclusive verdict instead of flapping.
		return nil
	}
	return err
}
