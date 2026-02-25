package manager //nolint:testpackage // tests access unexported manager type

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
)

type ManagerSuite struct {
	suite.Suite
	ctx context.Context
}

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}

func (s *ManagerSuite) SetupTest() {
	s.ctx = context.Background()
}

func (s *ManagerSuite) TestAddGetRemovePoolTable() {
	testCases := []struct {
		name      string
		ref       string
		addNil    bool
		wantFound bool
		checkRef  string
	}{
		{
			name:      "default reference when empty",
			ref:       "",
			addNil:    false,
			wantFound: true,
			checkRef:  datastore.DefaultPoolName,
		},
		{
			name:      "custom reference",
			ref:       "analytics",
			addNil:    false,
			wantFound: true,
			checkRef:  "analytics",
		},
		{
			name:      "nil pool ignored",
			ref:       "ignored",
			addNil:    true,
			wantFound: false,
			checkRef:  "ignored",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			mgrIntf, err := NewManager(s.ctx)
			s.Require().NoError(err)
			mgr := mgrIntf.(*manager)

			var pl pool.Pool
			if !tc.addNil {
				pl = pool.NewPool(s.ctx)
			}

			mgr.AddPool(s.ctx, tc.ref, pl)
			got := mgr.GetPool(s.ctx, tc.checkRef)
			if tc.wantFound {
				s.NotNil(got)
			} else {
				s.Nil(got)
			}

			mgr.RemovePool(s.ctx, tc.checkRef)
			s.Nil(mgr.GetPool(s.ctx, tc.checkRef))
		})
	}
}

func (s *ManagerSuite) TestDBAndMigrationNilPoolPaths() {
	mgrIntf, err := NewManager(s.ctx)
	s.Require().NoError(err)
	mgr := mgrIntf.(*manager)

	s.Nil(mgr.DB(s.ctx, false))
	s.Nil(mgr.DBWithPool(s.ctx, "missing", true))
	s.NoError(mgr.SaveMigration(s.ctx, nil))
	s.NoError(mgr.Migrate(s.ctx, nil, ""))
	mgr.Close(s.ctx)
}

func (s *ManagerSuite) TestGetPoolTypeGuard() {
	mgrIntf, err := NewManager(s.ctx)
	s.Require().NoError(err)
	mgr := mgrIntf.(*manager)

	mgr.dbPools.Store("bad", "not-a-pool")
	s.Nil(mgr.GetPool(s.ctx, "bad"))
}
