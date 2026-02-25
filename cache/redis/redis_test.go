package redis //nolint:testpackage // tests access package internals

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	testvalkey "github.com/pitabwire/frame/frametests/deps/testvalkey"
)

type RedisSuite struct {
	frametests.FrameBaseTestSuite
	dsn data.DSN
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func (s *RedisSuite) SetupSuite() {
	s.InitResourceFunc = func(_ context.Context) []definition.TestResource {
		return []definition.TestResource{testvalkey.New()}
	}
	s.FrameBaseTestSuite.SetupSuite()

	for _, dep := range s.Resources() {
		ds := dep.GetDS(s.T().Context())
		if ds.IsCache() {
			s.dsn = ds
			break
		}
	}
	s.Require().NotEmpty(s.dsn.String())
}

func (s *RedisSuite) TestNewAndOperationsTable() {
	ctx := context.Background()

	_, err := New(cache.WithDSN("://bad-dsn"))
	s.Require().Error(err)

	raw, err := New(cache.WithDSN(s.dsn), cache.WithMaxAge(2*time.Second))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = raw.Close() })

	testCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "set get exists delete",
			run: func() error {
				if err = raw.Set(ctx, "redis:key:1", []byte("value"), 0); err != nil {
					return err
				}
				val, found, getErr := raw.Get(ctx, "redis:key:1")
				s.True(found)
				s.Equal([]byte("value"), val)
				if getErr != nil {
					return getErr
				}
				exists, existsErr := raw.Exists(ctx, "redis:key:1")
				s.True(exists)
				if existsErr != nil {
					return existsErr
				}
				return raw.Delete(ctx, "redis:key:1")
			},
		},
		{
			name: "expire",
			run: func() error {
				if err = raw.Set(ctx, "redis:key:2", []byte("value"), time.Minute); err != nil {
					return err
				}
				if err = raw.Expire(ctx, "redis:key:2", time.Second); err != nil {
					return err
				}
				time.Sleep(1200 * time.Millisecond)
				_, found, getErr := raw.Get(ctx, "redis:key:2")
				s.False(found)
				return getErr
			},
		},
		{
			name: "increment decrement",
			run: func() error {
				if delErr := raw.Delete(ctx, "redis:counter"); delErr != nil {
					return delErr
				}
				val, incErr := raw.Increment(ctx, "redis:counter", 4)
				s.Equal(int64(4), val)
				if incErr != nil {
					return incErr
				}
				val, decErr := raw.Decrement(ctx, "redis:counter", 2)
				s.Equal(int64(2), val)
				return decErr
			},
		},
		{
			name: "flush",
			run: func() error {
				if err = raw.Set(ctx, "redis:flush", []byte("x"), 0); err != nil {
					return err
				}
				if err = raw.Flush(ctx); err != nil {
					return err
				}
				exists, existsErr := raw.Exists(ctx, "redis:flush")
				s.False(exists)
				return existsErr
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Require().NoError(tc.run())
			s.True(raw.SupportsPerKeyTTL())
		})
	}
}
