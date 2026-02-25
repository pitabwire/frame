package ratelimiter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/cache/jetstreamkv"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/ratelimiter"
)

type RateLimiterIntegrationSuite struct {
	frametests.FrameBaseTestSuite
	natsDSN data.DSN
}

func (s *RateLimiterIntegrationSuite) SetupSuite() {
	s.InitResourceFunc = func(_ context.Context) []definition.TestResource {
		return []definition.TestResource{
			testnats.New(),
		}
	}

	s.FrameBaseTestSuite.SetupSuite()

	for _, dep := range s.Resources() {
		ds := dep.GetDS(s.T().Context())
		if ds.IsQueue() {
			s.natsDSN = ds
			break
		}
	}

	require.NotEmpty(s.T(), s.natsDSN.String())
}

func TestRateLimiterIntegrationSuite(t *testing.T) {
	suite.Run(t, new(RateLimiterIntegrationSuite))
}

func (s *RateLimiterIntegrationSuite) TestWindowLimiterBackendSupportTable() {
	ctx := s.T().Context()

	testCases := []struct {
		name    string
		build   func() (cache.RawCache, error)
		wantErr error
	}{
		{
			name: "inmemory_supports_per_key_ttl",
			build: func() (cache.RawCache, error) {
				return cache.NewInMemoryCache(), nil
			},
			wantErr: nil,
		},
		{
			name: "jetstreamkv_rejected_for_window_limiter",
			build: func() (cache.RawCache, error) {
				return jetstreamkv.New(
					cache.WithDSN(s.natsDSN),
					cache.WithName("ratelimiter_"+util.RandomAlphaNumericString(8)),
					cache.WithMaxAge(time.Minute),
				)
			},
			wantErr: ratelimiter.ErrCacheDoesNotSupportPerKeyTTL,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			raw, err := tc.build()
			require.NoError(s.T(), err)
			defer raw.Close()

			_, gotErr := ratelimiter.NewWindowLimiter(raw, &ratelimiter.WindowConfig{
				WindowDuration: time.Minute,
				MaxPerWindow:   5,
				KeyPrefix:      "integration",
				FailOpen:       false,
			})

			if tc.wantErr == nil {
				assert.NoError(s.T(), gotErr)
				return
			}

			assert.Error(s.T(), gotErr)
			assert.True(s.T(), errors.Is(gotErr, tc.wantErr), gotErr)

			// Keep backend active long enough to verify interface behavior in live run.
			_, _ = raw.Exists(ctx, "nonexistent")
		})
	}
}
