package data

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DSNSuite struct {
	suite.Suite
}

func TestDSNSuite(t *testing.T) {
	suite.Run(t, new(DSNSuite))
}

func (s *DSNSuite) TestClassificationAndArray() {
	testCases := []struct {
		name      string
		dsn       DSN
		isMySQL   bool
		isPG      bool
		isDB      bool
		isRedis   bool
		isCache   bool
		isQueue   bool
		isNats    bool
		isMem     bool
		arraySize int
		valid     bool
	}{
		{
			name:      "postgres",
			dsn:       "postgres://user:pass@localhost:5432/db",
			isPG:      true,
			isDB:      true,
			valid:     true,
			arraySize: 1,
		},
		{
			name:      "mysql",
			dsn:       "mysql://localhost:3306/db",
			isMySQL:   true,
			isDB:      true,
			valid:     true,
			arraySize: 1,
		},
		{
			name:      "redis",
			dsn:       "redis://127.0.0.1:6379",
			isRedis:   true,
			isCache:   true,
			valid:     true,
			arraySize: 1,
		},
		{
			name:      "nats",
			dsn:       "nats://127.0.0.1:4222",
			isQueue:   true,
			isNats:    true,
			valid:     true,
			arraySize: 1,
		},
		{
			name:      "mem",
			dsn:       "mem://queue",
			isQueue:   true,
			isMem:     true,
			valid:     true,
			arraySize: 1,
		},
		{
			name:      "list",
			dsn:       "redis://a,redis://b",
			isRedis:   true,
			isCache:   true,
			valid:     true,
			arraySize: 2,
		},
		{
			name:      "invalid",
			dsn:       "    ",
			arraySize: 1,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Equal(tc.isMySQL, tc.dsn.IsMySQL())
			s.Equal(tc.isPG, tc.dsn.IsPostgres())
			s.Equal(tc.isDB, tc.dsn.IsDB())
			s.Equal(tc.isRedis, tc.dsn.IsRedis())
			s.Equal(tc.isCache, tc.dsn.IsCache())
			s.Equal(tc.isQueue, tc.dsn.IsQueue())
			s.Equal(tc.isNats, tc.dsn.IsNats())
			s.Equal(tc.isMem, tc.dsn.IsMem())
			s.Equal(tc.valid, tc.dsn.Valid())
			s.Len(tc.dsn.ToArray(), tc.arraySize)
		})
	}
}

func (s *DSNSuite) TestMutationHelpers() {
	dsn := DSN("https://user:pass@example.com:8443/base?p=1")

	s.Equal("https://user:pass@example.com:8443/base/v1?p=1", dsn.ExtendPath("v1").String())
	s.Equal("https://user:pass@example.com:8443/base-tail?p=1", dsn.SuffixPath("-tail").String())
	s.Equal("https://user:pass@example.com:8443/pre-/base?p=1", dsn.PrefixPath("/pre-").String())
	s.Equal("https://user:pass@example.com:8443?p=1", dsn.DelPath().String())
	s.Equal("2", dsn.ExtendQuery("p", "2").GetQuery("p"))
	s.Equal("", dsn.RemoveQuery("p").GetQuery("p"))
	s.Equal(dsn.String(), dsn.RemoveQuery("missing").String())

	pathSet, err := dsn.WithPath("/x")
	s.Require().NoError(err)
	s.Equal("/x", mustURI(pathSet).Path)

	pathSuffix, err := pathSet.WithPathSuffix("-y")
	s.Require().NoError(err)
	s.Equal("/x-y", mustURI(pathSuffix).Path)

	withQuery, err := dsn.WithQuery("a=1&b=2")
	s.Require().NoError(err)
	s.Equal("1", withQuery.GetQuery("a"))
	s.Equal("2", withQuery.GetQuery("b"))

	withUser, err := dsn.WithUser("alice")
	s.Require().NoError(err)
	s.Equal("alice", mustURI(withUser).User.Username())

	withPass, err := dsn.WithPassword("secret")
	s.Require().NoError(err)
	userInfo, ok := mustURI(withPass).User.(interface{ Password() (string, bool) })
	s.Require().True(ok)
	_, hasPass := userInfo.Password()
	s.True(hasPass)

	withBoth, err := dsn.WithUserAndPassword("bob", "pw")
	s.Require().NoError(err)
	s.Equal("bob", mustURI(withBoth).User.Username())

	changedPort, err := dsn.ChangePort("9443")
	s.Require().NoError(err)
	s.Equal(fmt.Sprintf("example.com:%s", "9443"), mustURI(changedPort).Host)
}

func (s *DSNSuite) TestErrorBranchesOnInvalidURI() {
	bad := DSN("://invalid")

	s.Equal(bad.String(), bad.ExtendPath("x").String())
	s.Equal(bad.String(), bad.SuffixPath("x").String())
	s.Equal(bad.String(), bad.PrefixPath("x").String())
	s.Equal(bad.String(), bad.DelPath().String())
	s.Equal(bad.String(), bad.ExtendQuery("a", "b").String())
	s.Equal(bad.String(), bad.RemoveQuery("a").String())
	s.Equal("", bad.GetQuery("a"))

	_, err := bad.WithPath("/x")
	s.Error(err)
	_, err = bad.WithPathSuffix("x")
	s.Error(err)
	_, err = bad.WithQuery("a=1")
	s.Error(err)
	_, err = bad.WithUser("u")
	s.Error(err)
	_, err = bad.WithPassword("p")
	s.Error(err)
	_, err = bad.WithUserAndPassword("u", "p")
	s.Error(err)
	_, err = bad.ChangePort("1")
	s.Error(err)
}

func mustURI(d DSN) *urlParts {
	u, _ := d.ToURI()
	return &urlParts{Path: u.Path, Host: u.Host, User: u.User}
}

type urlParts struct {
	Path string
	Host string
	User interface {
		Username() string
		Password() (string, bool)
	}
}
