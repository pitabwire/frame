package jetstreamkv //nolint:testpackage // tests access unexported internals

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
)

type JetstreamSuite struct {
	frametests.FrameBaseTestSuite
	dsn data.DSN
}

func TestJetstreamSuite(t *testing.T) {
	suite.Run(t, new(JetstreamSuite))
}

func (s *JetstreamSuite) SetupSuite() {
	s.InitResourceFunc = func(_ context.Context) []definition.TestResource {
		return []definition.TestResource{testnats.New()}
	}
	s.FrameBaseTestSuite.SetupSuite()

	for _, dep := range s.Resources() {
		ds := dep.GetDS(s.T().Context())
		if ds.IsQueue() {
			s.dsn = ds
			break
		}
	}
	s.Require().NotEmpty(s.dsn.String())
}

func (s *JetstreamSuite) TestNewAndOperationsTable() {
	ctx := context.Background()

	_, err := New(cache.WithDSN("://bad-dsn"))
	s.Require().Error(err)

	raw, err := New(cache.WithDSN(s.dsn), cache.WithName("cache_a"), cache.WithMaxAge(time.Minute))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = raw.Close() })

	// Re-open same bucket to cover the stream-in-use branch.
	raw2, err := New(cache.WithDSN(s.dsn), cache.WithName("cache_a"), cache.WithMaxAge(time.Minute))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = raw2.Close() })

	testCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "set get exists delete",
			run: func() error {
				if err = raw.Set(ctx, "js_key_1", []byte("value"), time.Second); err != nil {
					return err
				}
				val, found, getErr := raw.Get(ctx, "js_key_1")
				s.True(found)
				s.Equal([]byte("value"), val)
				if getErr != nil {
					return getErr
				}
				exists, existsErr := raw.Exists(ctx, "js_key_1")
				s.True(exists)
				if existsErr != nil {
					return existsErr
				}
				return raw.Delete(ctx, "js_key_1")
			},
		},
		{
			name: "increment decrement",
			run: func() error {
				_ = raw.Delete(ctx, "js_counter")
				val, incErr := raw.Increment(ctx, "js_counter", 5)
				s.Equal(int64(5), val)
				if incErr != nil {
					return incErr
				}
				val, decErr := raw.Decrement(ctx, "js_counter", 2)
				s.Equal(int64(3), val)
				return decErr
			},
		},
		{
			name: "increment parse error",
			run: func() error {
				if err = raw.Set(ctx, "js_counter_bad", []byte("not-int"), 0); err != nil {
					return err
				}
				_, incErr := raw.Increment(ctx, "js_counter_bad", 1)
				s.Error(incErr)
				return nil
			},
		},
		{
			name: "flush",
			run: func() error {
				if err = raw.Set(ctx, "js_flush", []byte("x"), 0); err != nil {
					return err
				}
				if err = raw.Flush(ctx); err != nil {
					return err
				}
				exists, existsErr := raw.Exists(ctx, "js_flush")
				s.False(exists)
				return existsErr
			},
		},
		{
			name: "expire noop",
			run: func() error {
				s.False(raw.SupportsPerKeyTTL())
				return raw.Expire(ctx, "js_key_missing", time.Second)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Require().NoError(tc.run())
		})
	}
}

func (s *JetstreamSuite) TestRevisionConflictCheck() {
	c := &Cache{}
	s.False(c.isRevisionConflict(nil))
	s.False(c.isRevisionConflict(&nats.APIError{ErrorCode: 10001}))
	s.True(c.isRevisionConflict(&nats.APIError{ErrorCode: nats.JSErrCodeStreamWrongLastSequence}))
}
