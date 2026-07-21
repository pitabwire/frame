package postgres_test

import (
	"context"
	"testing"

	"github.com/pitabwire/frame/v2/data"
	"github.com/pitabwire/frame/v2/datastore/dialect"
	dialectpg "github.com/pitabwire/frame/v2/datastore/dialect/postgres"
	"github.com/pitabwire/frame/v2/frametests/definition"
	"github.com/pitabwire/frame/v2/tenancy"
	tenpg "github.com/pitabwire/frame/v2/tenancy/postgres"
	"github.com/pitabwire/frame/v2/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
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

func seedTwoTenants(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&rlsEntity{
		BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
		Name:      "row-T1",
	}).Error)
	require.NoError(t, db.Create(&rlsEntity{
		BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
		Name:      "row-T2",
	}).Error)
}

func installRLS(ctx context.Context, t *testing.T, env *providerEnv) {
	t.Helper()
	require.NoError(t, env.adminDB.AutoMigrate(&rlsEntity{}))
	require.NoError(t, env.adminDB.Exec("TRUNCATE rls_entities").Error)
	require.NoError(t, env.prov.Install(ctx, env.adminDB, []tenancy.ModelInfo{
		{Table: "rls_entities", TenantColumn: "tenant_id", PartitionColumn: "partition_id"},
	}))
	grantRLSEntitiesAccess(t, env.adminDB)
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
		installRLS(ctx, t, env)
		seedTwoTenants(t, env.adminDB)

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
		installRLS(ctx, t, env)

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

func (s *ProviderTestSuite) TestNoClaimsDoesNotErrorAndSeesAll() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()
		installRLS(ctx, t, env)
		seedTwoTenants(t, env.adminDB)

		// Missing claims: no error, no filtering.
		var got []rlsEntity
		require.NoError(t, env.scopedDB.WithContext(ctx).Find(&got).Error)
		require.Len(t, got, 2)
	})
}

func (s *ProviderTestSuite) TestSkipClaimsBypassEnforcement() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()
		installRLS(ctx, t, env)
		seedTwoTenants(t, env.adminDB)

		// Skip=true should make every row visible (provider does not
		// push any session scope; empty-match-all branch fires).
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
		installRLS(ctx, t, env)
		seedTwoTenants(t, env.adminDB)

		// Bind claims and issue a no-op query so the hook fires.
		ctxScoped := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1"},
		})
		require.NoError(t, env.scopedDB.WithContext(ctxScoped).Exec("SELECT 1").Error)

		// Subsequent acquire with no claims must see empty session vars
		// and therefore match-all again.
		var got string
		require.NoError(t, env.scopedDB.Raw(
			`SELECT COALESCE(current_setting('app.tenant_id', true), '')`,
		).Scan(&got).Error)
		require.Empty(t, got, "session state must be reset by AfterRelease")

		var rows []rlsEntity
		require.NoError(t, env.scopedDB.WithContext(ctx).Find(&rows).Error)
		require.Len(t, rows, 2, "unscoped after release must see all rows")
	})
}

func (s *ProviderTestSuite) TestNoCrossTenantLeakOnConnReuse() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()
		installRLS(ctx, t, env)
		seedTwoTenants(t, env.adminDB)

		// Rapidly alternate tenants on the same pool — each acquire must
		// fully replace session state.
		for i := range 20 {
			var cctx context.Context
			wantName := "row-T1"
			if i%2 == 0 {
				cctx = tenancy.WithClaims(ctx, &tenancy.Claims{TenantID: "T1", PartitionIDs: []string{"P1"}})
			} else {
				cctx = tenancy.WithClaims(ctx, &tenancy.Claims{TenantID: "T2", PartitionIDs: []string{"P2"}})
				wantName = "row-T2"
			}
			var got []rlsEntity
			require.NoError(t, env.scopedDB.WithContext(cctx).Find(&got).Error)
			require.Len(t, got, 1, "iteration %d", i)
			require.Equal(t, wantName, got[0].Name)
		}
	})
}

func (s *ProviderTestSuite) TestWithCheckRejectsCrossTenantInsert() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()
		installRLS(ctx, t, env)

		ctxT1 := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1"},
		})

		// Insert for own tenant/partition succeeds.
		require.NoError(t, env.scopedDB.WithContext(ctxT1).Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P1"},
			Name:      "own-row",
		}).Error)

		// Insert for another tenant must fail WITH CHECK.
		err := env.scopedDB.WithContext(ctxT1).Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T2", PartitionID: "P2"},
			Name:      "cross-tenant",
		}).Error
		require.Error(t, err, "WITH CHECK must reject cross-tenant insert")

		// Insert for own tenant but wrong partition must fail.
		err = env.scopedDB.WithContext(ctxT1).Create(&rlsEntity{
			BaseModel: data.BaseModel{TenantID: "T1", PartitionID: "P9"},
			Name:      "wrong-partition",
		}).Error
		require.Error(t, err, "WITH CHECK must reject out-of-partition insert")
	})
}

func (s *ProviderTestSuite) TestCommaInPartitionIDRejectsAcquire() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()
		installRLS(ctx, t, env)

		ctxBad := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1,P2"},
		})
		var got []rlsEntity
		err := env.scopedDB.WithContext(ctxBad).Find(&got).Error
		require.Error(t, err)
		require.Contains(t, err.Error(), "contains ','")
	})
}

func (s *ProviderTestSuite) TestTransactionKeepsConsistentScope() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		dsn := dep.ByIsDatabase(ctx).GetDS(ctx).String()

		env := s.providerSetup(ctx, t, dsn)
		defer env.cleanup()
		installRLS(ctx, t, env)
		seedTwoTenants(t, env.adminDB)

		ctxT1 := tenancy.WithClaims(ctx, &tenancy.Claims{
			TenantID:     "T1",
			PartitionIDs: []string{"P1"},
		})

		err := env.scopedDB.WithContext(ctxT1).Transaction(func(tx *gorm.DB) error {
			var got []rlsEntity
			if err := tx.Find(&got).Error; err != nil {
				return err
			}
			require.Len(t, got, 1)
			require.Equal(t, "row-T1", got[0].Name)

			got = nil
			if err := tx.Find(&got).Error; err != nil {
				return err
			}
			require.Len(t, got, 1)
			return nil
		})
		require.NoError(t, err)
	})
}
