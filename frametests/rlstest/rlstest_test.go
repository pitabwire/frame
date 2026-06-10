// Copyright 2023-2026 Peter Bwire
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rlstest_test

import (
	"context"
	"testing"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/rlstest"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/tenancy"
	"github.com/pitabwire/frame/tests"
)

type RLSTestSuite struct {
	tests.BaseTestSuite
}

func TestRLSTestSuite(t *testing.T) {
	suite.Run(t, &RLSTestSuite{})
}

type rlsWidget struct {
	data.BaseModel
	Name string `gorm:"type:varchar(64)"`
}

func (rlsWidget) TableName() string { return "rls_widgets" }

func tenantCtx(tenantID, partitionID string) context.Context {
	claims := &security.AuthenticationClaims{TenantID: tenantID, PartitionID: partitionID}
	claims.Subject = "user-" + tenantID
	return claims.ClaimsToContext(context.Background())
}

// The wrapper must keep migrations on the superuser path, then — once
// Enable is called — drop every acquired connection to the unprivileged
// role so FORCE ROW LEVEL SECURITY actually filters cross-tenant reads.
func (s *RLSTestSuite) TestEnableGatesRoleDrop() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx := t.Context()
		// Randomised database per dependency permutation — the harness
		// runs this body once per option set against shared containers.
		randomDS, cleanup, err := dep.ByIsDatabase(ctx).GetRandomisedDS(ctx, util.RandomAlphaNumericString(8))
		require.NoError(t, err)
		t.Cleanup(func() { cleanup(ctx) })
		dsn := randomDS.String()

		require.NoError(t, rlstest.CreateRole(ctx, dsn))

		prov := rlstest.New()
		p := pool.NewPool(ctx, pool.WithTenancyProvider(prov))
		require.NoError(t, p.AddConnection(ctx,
			pool.WithConnection(dsn, false),
			pool.WithPreparedStatements(false),
		))
		t.Cleanup(func() { p.Close(ctx) })

		adminDB := p.DB(ctx, false)
		require.NoError(t, adminDB.AutoMigrate(&rlsWidget{}))
		enrolled, enrollErr := tenancy.EnrolledModels(adminDB, []any{&rlsWidget{}})
		require.NoError(t, enrollErr)
		require.NoError(t, prov.Install(ctx, adminDB, enrolled))
		require.NoError(t, rlstest.GrantAll(ctx, dsn))

		// Seed one row under tenant A (still superuser — Enable not called).
		ctxA := tenantCtx("tenant-a", "part-a")
		ctxB := tenantCtx("tenant-b", "part-b")
		require.NoError(t, p.DB(ctxA, false).Create(&rlsWidget{Name: "a-thing"}).Error)

		// Before Enable: superuser bypasses FORCE RLS, tenant B sees the row.
		var leaked []rlsWidget
		require.NoError(t, p.DB(ctxB, false).Find(&leaked).Error)
		require.Len(t, leaked, 1, "superuser must bypass RLS before Enable — gate proves the test setup matters")

		prov.Enable()

		// After Enable: tenant B's connection drops to the unprivileged
		// role; the app_tenancy_isolation policy filters tenant A's row.
		var isolated []rlsWidget
		require.NoError(t, p.DB(ctxB, false).Find(&isolated).Error)
		require.Empty(t, isolated, "cross-tenant read must return empty under RLS")

		// Tenant A still sees its own row through the restricted role.
		var own []rlsWidget
		require.NoError(t, p.DB(ctxA, false).Find(&own).Error)
		require.Len(t, own, 1)

		// Claim-less context (system path) keeps match-all semantics.
		var all []rlsWidget
		require.NoError(t, p.DB(context.Background(), false).Find(&all).Error)
		require.Len(t, all, 1)
	})
}
