package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pitabwire/frame/v3/datastore/dialect"
	dialectpg "github.com/pitabwire/frame/v3/datastore/dialect/postgres"
	"github.com/pitabwire/frame/v3/tenancy"
	tenpg "github.com/pitabwire/frame/v3/tenancy/postgres"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

const (
	// Images pinned by digest so the pooler behaviour under test is fixed.
	affinityPostgresImage = "postgres:17-alpine@sha256:" +
		"f9cf3e1f0066f0eb0c48707d864f0ac5941e9ff454a072077d97a4d278d07d73"
	// PgBouncer 1.26.1.
	affinityPgBouncerImage = "edoburu/pgbouncer@sha256:" +
		"8638aa563607256d3c7440437534195f1066a92b68015d89ae9d6b935b444a6d"

	affinityUser     = "affinity"
	affinityPassword = "affinity-pw"
	affinityDB       = "affinity"
	affinityPGAlias  = "pg"
)

// SessionAffinitySuite runs Postgres plus PgBouncer in transaction and in
// session pool mode, and proves CheckSessionAffinity tells them apart.
type SessionAffinitySuite struct {
	suite.Suite

	direct      string
	txnPooler   string
	sessPooler  string
	terminators []func()
}

func TestSessionAffinitySuite(t *testing.T) {
	suite.Run(t, new(SessionAffinitySuite))
}

func (s *SessionAffinitySuite) SetupSuite() {
	ctx := s.T().Context()

	nw, err := network.New(ctx)
	s.Require().NoError(err)
	s.terminators = append(s.terminators, func() { _ = nw.Remove(context.Background()) })

	pg, err := tcPostgres.Run(ctx, affinityPostgresImage,
		tcPostgres.WithDatabase(affinityDB),
		tcPostgres.WithUsername(affinityUser),
		tcPostgres.WithPassword(affinityPassword),
		network.WithNetwork([]string{affinityPGAlias}, nw),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	s.Require().NoError(err)
	s.terminators = append(s.terminators, func() { _ = pg.Terminate(context.Background()) })

	s.direct, err = pg.ConnectionString(ctx, "sslmode=disable")
	s.Require().NoError(err)
	s.txnPooler = s.startPgBouncer(ctx, nw, "transaction")
	s.sessPooler = s.startPgBouncer(ctx, nw, "session")
}

func (s *SessionAffinitySuite) startPgBouncer(
	ctx context.Context,
	nw *testcontainers.DockerNetwork,
	mode string,
) string {
	c, err := testcontainers.Run(ctx, affinityPgBouncerImage,
		network.WithNetwork(nil, nw),
		testcontainers.WithEnv(map[string]string{
			"DB_HOST":           affinityPGAlias,
			"DB_USER":           affinityUser,
			"DB_PASSWORD":       affinityPassword,
			"DB_NAME":           affinityDB,
			"AUTH_TYPE":         "scram-sha-256",
			"POOL_MODE":         mode,
			"DEFAULT_POOL_SIZE": "10",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)),
	)
	s.Require().NoError(err)
	s.terminators = append(s.terminators, func() { _ = c.Terminate(context.Background()) })

	host, err := c.Host(ctx)
	s.Require().NoError(err)
	port, err := c.MappedPort(ctx, "5432/tcp")
	s.Require().NoError(err)
	// Simple protocol: transaction poolers cannot hold prepared statements.
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&default_query_exec_mode=simple_protocol",
		affinityUser, affinityPassword, net.JoinHostPort(host, port.Port()), affinityDB)
}

func (s *SessionAffinitySuite) TearDownSuite() {
	for i := len(s.terminators) - 1; i >= 0; i-- {
		s.terminators[i]()
	}
}

// openTenancyDB opens dsn exactly like Frame does for a tenancy-enforced
// pool: the dialect adapter with the RLS provider's acquire/release hooks.
func (s *SessionAffinitySuite) openTenancyDB(dsn string, mode tenancy.SecurityMode) *gorm.DB {
	ctx := s.T().Context()
	adapter := dialectpg.New()
	prov := tenpg.New(tenpg.WithSecurityMode(mode))
	s.Require().NoError(prov.WireAdapter(adapter))
	dialector, _, closeFn, err := adapter.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 8})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = closeFn() })
	// Secure modes reject unscoped conns, so skip gorm's unscoped ping.
	db, err := gorm.Open(dialector, &gorm.Config{DisableAutomaticPing: true})
	s.Require().NoError(err)
	return db
}

func (s *SessionAffinitySuite) TestCheckSessionAffinity() {
	cases := []struct {
		name    string
		dsn     func() string
		mode    tenancy.SecurityMode
		wantErr bool
	}{
		{name: "direct postgres keeps session state", dsn: func() string { return s.direct },
			mode: tenancy.ModeFailClosed},
		{name: "pgbouncer session mode keeps session state", dsn: func() string { return s.sessPooler },
			mode: tenancy.ModeHybrid},
		{name: "pgbouncer transaction mode loses session state", dsn: func() string { return s.txnPooler },
			mode: tenancy.ModeFailClosed, wantErr: true},
		{name: "detected under fail-open too (caller decides policy)", dsn: func() string { return s.txnPooler },
			mode: tenancy.ModeFailOpen, wantErr: true},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			db := s.openTenancyDB(tc.dsn(), tc.mode)
			// Repeat: the verdict must be deterministic, not luck of scheduling.
			for range 5 {
				err := tenpg.CheckSessionAffinity(s.T().Context(), db)
				if tc.wantErr {
					s.Require().ErrorIs(err, tenpg.ErrSessionAffinityRequired)
				} else {
					s.Require().NoError(err)
				}
			}
		})
	}
}

func (s *SessionAffinitySuite) TestSessionAffinityHealthCheck() {
	healthy := &tenpg.SessionAffinityCheck{DB: func(context.Context) *gorm.DB {
		return s.openTenancyDB(s.direct, tenancy.ModeFailClosed)
	}}
	s.Equal("tenancy_session_affinity", healthy.Name())
	s.Require().NoError(healthy.CheckHealth())

	pooled := &tenpg.SessionAffinityCheck{DB: func(context.Context) *gorm.DB {
		return s.openTenancyDB(s.txnPooler, tenancy.ModeFailClosed)
	}}
	s.Require().ErrorIs(pooled.CheckHealth(), tenpg.ErrSessionAffinityRequired,
		"readiness must fail closed behind a transaction pooler")

	missing := &tenpg.SessionAffinityCheck{}
	s.Require().Error(missing.CheckHealth())
}

// Demonstrates the hazard the guard exists for: behind a transaction pooler
// the tenancy GUC bound at acquire is not what a later statement on the same
// client connection sees.
func (s *SessionAffinitySuite) TestTransactionPoolerLosesTenancyBinding() {
	db := s.openTenancyDB(s.txnPooler, tenancy.ModeFailClosed)
	sqlDB, err := db.DB()
	s.Require().NoError(err)
	ctx := s.T().Context()

	ctxA := tenancy.WithClaims(ctx, &tenancy.Claims{TenantID: "tenant-A", PartitionIDs: []string{"p"}})
	ctxB := tenancy.WithClaims(ctx, &tenancy.Claims{TenantID: "tenant-B", PartitionIDs: []string{"p"}})

	connA, err := sqlDB.Conn(ctxA)
	s.Require().NoError(err)
	defer connA.Close()
	connB, err := sqlDB.Conn(ctxB)
	s.Require().NoError(err)
	defer connB.Close()
	_, err = connB.ExecContext(ctxB, "BEGIN")
	s.Require().NoError(err)
	defer func() { _, _ = connB.ExecContext(ctxB, "ROLLBACK") }()

	var seen sql.NullString
	s.Require().NoError(connA.QueryRowContext(ctxA, "SELECT current_setting('app.tenant_id', true)").Scan(&seen))
	s.NotEqual("tenant-A", seen.String,
		"transaction pooling hands A's next statement a backend without A's binding")
}
