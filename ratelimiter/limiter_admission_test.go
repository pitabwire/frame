package ratelimiter_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/ratelimiter"
)

func TestConcurrencyLimiterTryAcquireAndRelease(t *testing.T) {
	limiter, err := ratelimiter.NewConcurrencyLimiter(2)
	require.NoError(t, err)

	p1, ok := limiter.TryAcquire()
	require.True(t, ok)
	p2, ok := limiter.TryAcquire()
	require.True(t, ok)
	_, ok = limiter.TryAcquire()
	require.False(t, ok)

	assert.Equal(t, 2, limiter.InFlight())
	assert.Equal(t, 0, limiter.Available())

	p1.Release()
	assert.Equal(t, 1, limiter.InFlight())
	assert.Equal(t, 1, limiter.Available())

	p2.Release()
	assert.Equal(t, 0, limiter.InFlight())
}

func TestConcurrencyLimiterAcquireHonorsContext(t *testing.T) {
	limiter, err := ratelimiter.NewConcurrencyLimiter(1)
	require.NoError(t, err)

	permit, err := limiter.Acquire(context.Background())
	require.NoError(t, err)
	defer permit.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = limiter.Acquire(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestConcurrencyLimiterExecuteReleasesPermit(t *testing.T) {
	limiter, err := ratelimiter.NewConcurrencyLimiter(1)
	require.NoError(t, err)

	err = limiter.Execute(context.Background(), func(context.Context) error {
		assert.Equal(t, 1, limiter.InFlight())
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, limiter.InFlight())
}

func TestQueueDepthLimiterRejectsAndResumesWithHysteresis(t *testing.T) {
	var depth atomic.Int64
	depth.Store(5)

	limiter, err := ratelimiter.NewQueueDepthLimiter(
		func(context.Context) (int64, error) {
			return depth.Load(), nil
		},
		ratelimiter.QueueDepthConfig{
			RejectAtDepth:   10,
			ResumeAtDepth:   4,
			RefreshInterval: time.Millisecond,
			FailOpen:        false,
		},
	)
	require.NoError(t, err)

	assert.True(t, limiter.Allow(context.Background()))

	depth.Store(12)
	time.Sleep(2 * time.Millisecond)
	assert.False(t, limiter.Allow(context.Background()))

	depth.Store(8)
	time.Sleep(2 * time.Millisecond)
	assert.False(t, limiter.Allow(context.Background()))

	depth.Store(4)
	time.Sleep(2 * time.Millisecond)
	assert.True(t, limiter.Allow(context.Background()))
}

func TestQueueDepthLimiterCachesDepthLookups(t *testing.T) {
	var calls atomic.Int64

	limiter, err := ratelimiter.NewQueueDepthLimiter(
		func(context.Context) (int64, error) {
			calls.Add(1)
			return 1, nil
		},
		ratelimiter.QueueDepthConfig{
			RejectAtDepth:   10,
			ResumeAtDepth:   5,
			RefreshInterval: time.Minute,
			FailOpen:        false,
		},
	)
	require.NoError(t, err)

	assert.True(t, limiter.Allow(context.Background()))
	assert.True(t, limiter.Allow(context.Background()))
	assert.True(t, limiter.Allow(context.Background()))
	assert.Equal(t, int64(1), calls.Load())
}

func TestQueueDepthLimiterFailOpenAndFailClosed(t *testing.T) {
	failErr := errors.New("depth lookup failed")

	failOpenLimiter, err := ratelimiter.NewQueueDepthLimiter(
		func(context.Context) (int64, error) { return 0, failErr },
		ratelimiter.QueueDepthConfig{
			RejectAtDepth:   10,
			ResumeAtDepth:   5,
			RefreshInterval: time.Millisecond,
			FailOpen:        true,
		},
	)
	require.NoError(t, err)
	assert.True(t, failOpenLimiter.Allow(context.Background()))

	failClosedLimiter, err := ratelimiter.NewQueueDepthLimiter(
		func(context.Context) (int64, error) { return 0, failErr },
		ratelimiter.QueueDepthConfig{
			RejectAtDepth:   10,
			ResumeAtDepth:   5,
			RefreshInterval: time.Millisecond,
			FailOpen:        false,
		},
	)
	require.NoError(t, err)
	assert.False(t, failClosedLimiter.Allow(context.Background()))
}
