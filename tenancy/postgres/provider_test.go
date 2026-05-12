package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/dialect"
	dialectpg "github.com/pitabwire/frame/datastore/dialect/postgres"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tenancy"
	tenpg "github.com/pitabwire/frame/tenancy/postgres"
	"github.com/pitabwire/frame/tests"
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

// rlsTestRole is the unprivileged role under which scoped queries run.
// The testcontainers postgres user is a superuser and therefore bypasses
// RLS even with FORCE — so the tests need to drop to a non-superuser to
// prove the policy actually filters rows.
const rlsTestRole = "rls_test_user"

// providerEnv is the per-subtest fixture. adminDB is opened as the
// container's superuser and is used for migration, RLS installation,
// and seeding. scopedDB is wired with the tenancy provider AND a
// test-only SET ROLE hook so scoped queries run as a non-superuser
// (the only way to exercise FORCE ROW LEVEL SECURITY).
type providerEnv struct {
	adminDB  *gorm.DB
	scopedDB *gorm.DB
	prov     *tenpg.Provider
	cleanup  func()
}

// providerSetup wires up the per-subtest fixture. The caller must
// defer env.cleanup().
func (s *ProviderTestSuite) providerSetup(ctx context.Context, t *testing.T, dsn string) *providerEnv {
	t.Helper()

	// adminAdapter: hookless. Used to migrate, install RLS, seed data,
	// and create the test role itself.
	adminAdapter := dialectpg.New()
	adminDialector, _, adminClose, err := adminAdapter.OpenConnection(ctx, dsn, dialect.ConnectionOptions{MaxOpen: 4})
	require.NoError(t, err)
	adminDB, err := gorm.Open(adminDialector, &gorm.Config{})
	require.NoError(t, err)

	// Create the unprivileged role (idempotent).
	require.NoError(t, adminDB.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '`+rlsTestRole+`') THEN
				CREATE ROLE `+rlsTestRole+` NOLOGIN;
			END IF;
		END $$;
	`).Error)

	// scopedAdapter: wired with the tenancy provider plus a SET ROLE
	// hook so every acquired conn drops to the test role. Hook ordering
	// matters — tenancy push runs first (so app.* vars belong to the
	// superuser context), SET ROLE second so the policy check runs as
	// the restricted role.
	scopedAdapter := dialectpg.New()
	prov := tenpg.New()
	require.NoError(t, prov.WireAdapter(scopedAdapter))
	require.NoError(t, scopedAdapter.RegisterAcquireHook(func(hookCtx context.Context, conn dialect.DialectConn) error {
		return conn.Exec(hookCtx, "SET ROLE "+rlsTestRole)
	}))
	require.NoError(t, scopedAdapter.RegisterReleaseHook(func(hookCtx context.Context, conn dialect.DialectConn) error {
		return conn.Exec(hookCtx, "RESET ROLE")
	}))

	scopedDialector, _, scopedClose, err := scopedAdapter.OpenConnection(
		ctx,
		dsn,
		dialect.ConnectionOptions{MaxOpen: 4},
	)
	require.NoError(t, err)
	scopedDB, err := gorm.Open(scopedDialector, &gorm.Config{})
	require.NoError(t, err)

	return &providerEnv{
		adminDB:  adminDB,
		scopedDB: scopedDB,
		prov:     prov,
		cleanup: func() {
			_ = scopedClose()
			_ = adminClose()
		},
	}
}

// grantRLSEntitiesAccess gives the test role enough privileges to read
// and write the rls_entities table. Called after AutoMigrate so the
// table exists.
func grantRLSEntitiesAccess(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(
		"GRANT SELECT, INSERT, UPDATE, DELETE ON rls_entities TO "+rlsTestRole,
	).Error)
}

func (s *ProviderTestSuite) TestInstallIdempotent() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()

		require.NoError(t, env.adminDB.AutoMigrate(&rlsEntity{}))

		models := []tenancy.ModelInfo{{
			Table:           "rls_entities",
			TenantColumn:    "tenant_id",
			PartitionColumn: "partition_id",
		}}
		require.NoError(t, env.prov.Install(ctx, env.adminDB, models), "first install")
		require.NoError(t, env.prov.Install(ctx, env.adminDB, models), "second install (idempotent)")

		// Verify the policy exists exactly once.
		var count int64
		require.NoError(t, env.adminDB.Raw(
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

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()

		require.NoError(t, env.adminDB.AutoMigrate(&rlsEntity{}))
		require.NoError(t, env.adminDB.Exec("TRUNCATE rls_entities").Error)
		require.NoError(t, env.prov.Install(ctx, env.adminDB, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))
		grantRLSEntitiesAccess(t, env.adminDB)

		// Seed across two tenants using the admin DB (no claims, RLS bypassed by superuser).
		require.NoError(t, env.adminDB.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
			Name:      "row-T1",
		}).Error)
		require.NoError(t, env.adminDB.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
			Name:      "row-T2",
		}).Error)

		// Bind T1 claims and query via the scoped DB — only T1's row should be visible.
		ctxT1 := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1"},
		})
		var got []rlsEntity
		require.NoError(t, env.scopedDB.WithContext(ctxT1).Find(&got).Error)
		require.Len(t, got, 1)
		require.Equal(t, "row-T1", got[0].Name)

		// Switch to T2 — only T2's row.
		ctxT2 := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T2",
			PartitionIDs: []string{"P2"},
		})
		got = nil
		require.NoError(t, env.scopedDB.WithContext(ctxT2).Find(&got).Error)
		require.Len(t, got, 1)
		require.Equal(t, "row-T2", got[0].Name)
	})
}

func (s *ProviderTestSuite) TestRLSMultiPartitionPrincipal() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()

		require.NoError(t, env.adminDB.AutoMigrate(&rlsEntity{}))
		require.NoError(t, env.adminDB.Exec("TRUNCATE rls_entities").Error)
		require.NoError(t, env.prov.Install(ctx, env.adminDB, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))
		grantRLSEntitiesAccess(t, env.adminDB)

		// Seed three rows for tenant T1 across three partitions.
		for _, p := range []string{"P1", "P2", "P3"} {
			require.NoError(t, env.adminDB.Create(&rlsEntity{
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
		require.NoError(t, env.scopedDB.WithContext(ctxMulti).Order("name").Find(&got).Error)
		require.Len(t, got, 2)
		require.Equal(t, "row-P1", got[0].Name)
		require.Equal(t, "row-P3", got[1].Name)
	})
}

func (s *ProviderTestSuite) TestSkipClaimsBypassEnforcement() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()

		require.NoError(t, env.adminDB.AutoMigrate(&rlsEntity{}))
		require.NoError(t, env.adminDB.Exec("TRUNCATE rls_entities").Error)
		require.NoError(t, env.prov.Install(ctx, env.adminDB, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))
		grantRLSEntitiesAccess(t, env.adminDB)

		require.NoError(t, env.adminDB.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
			Name:      "row-T1",
		}).Error)
		require.NoError(t, env.adminDB.Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
			Name:      "row-T2",
		}).Error)

		// Skip=true should make every row visible (provider does not
		// push any session vars; RLS empty-match-all branch fires).
		ctxSkip := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "anything",
			PartitionIDs: []string{"anything"},
			Skip:         true,
		})
		var got []rlsEntity
		require.NoError(t, env.scopedDB.WithContext(ctxSkip).Find(&got).Error)
		require.Len(t, got, 2)
	})
}

func (s *ProviderTestSuite) TestAfterReleaseResetsSessionState() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()

		require.NoError(t, env.adminDB.AutoMigrate(&rlsEntity{}))
		require.NoError(t, env.prov.Install(ctx, env.adminDB, []tenancy.ModelInfo{
			{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
		}))
		grantRLSEntitiesAccess(t, env.adminDB)

		// Bind claims and issue a no-op query so the hook fires.
		ctxScoped := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1"},
		})
		require.NoError(t, env.scopedDB.WithContext(ctxScoped).Exec("SELECT 1").Error)

		// Subsequent acquire with no claims must see empty session vars.
		// current_setting(..., true) returns NULL when the var has never
		// been set on this conn (after RESET), which is the post-release
		// invariant we want to assert. COALESCE keeps the column non-NULL
		// so a plain string scan works.
		var got string
		require.NoError(t, env.scopedDB.Raw(
			`SELECT COALESCE(current_setting('app.tenant_id', true), '')`,
		).Scan(&got).Error)
		require.Empty(t, got, "session state must be reset by AfterRelease")
	})
}
