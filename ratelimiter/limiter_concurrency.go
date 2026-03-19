package ratelimiter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrConcurrencyLimitReached is returned by callers that choose to convert a
// failed TryAcquire call into an error.
//
// The limiter itself exposes TryAcquire as a boolean because the common fast
// path is "acquired" vs "not acquired". This sentinel exists so services can
// map that condition into their own transport or business error model
// consistently.
var ErrConcurrencyLimitReached = errors.New("concurrency limit reached")

// ConcurrencyLimiter caps the number of in-flight operations in a single
// process.
//
// Use this limiter when the protected resource is local and finite. Typical
// examples include:
//
//   - connector execution slots
//   - CPU-heavy transforms
//   - datastore-heavy handlers
//   - local worker fan-out that would otherwise swamp memory, CPU, or sockets
//
// This limiter protects simultaneous work, not request rate. A service may
// process 10,000 requests per minute safely if only 20 run at once, yet fail at
// 200 requests per minute if all 200 execute concurrently. ConcurrencyLimiter is
// for the latter case.
//
// This limiter does NOT provide cross-process fairness. If ten replicas each
// have a limit of 100, the fleet-wide limit is effectively about 1,000. That is
// often exactly what you want for worker-side protection, but it is not
// suitable as a tenant-fair global quota.
//
// Use TryAcquire when callers should fail fast instead of waiting. Use Acquire
// when bounded waiting is correct and the caller can provide a context with a
// deadline or cancellation signal.
type ConcurrencyLimiter struct {
	limit    int64
	inFlight atomic.Int64
	permits  chan struct{}
}

// ConcurrencyPermit represents one acquired concurrency slot.
//
// The permit must be released exactly once. Release is idempotent so callers
// can safely defer it. The usual pattern is:
//
//	permit, err := limiter.Acquire(ctx)
//	if err != nil {
//	    return err
//	}
//	defer permit.Release()
type ConcurrencyPermit struct {
	limiter *ConcurrencyLimiter
	once    sync.Once
}

// NewConcurrencyLimiter creates a limiter with the given in-flight limit.
//
// The limit must reflect the capacity of one process, not the whole fleet. If
// callers need a fleet-wide cap, pair this limiter with a distributed admission
// control mechanism.
func NewConcurrencyLimiter(limit int) (*ConcurrencyLimiter, error) {
	if limit <= 0 {
		return nil, errors.New("concurrency limit must be greater than zero")
	}

	return &ConcurrencyLimiter{
		limit:   int64(limit),
		permits: make(chan struct{}, limit),
	}, nil
}

// Limit returns the configured maximum number of in-flight operations.
func (cl *ConcurrencyLimiter) Limit() int {
	if cl == nil {
		return 0
	}

	return int(cl.limit)
}

// InFlight returns the current number of acquired permits.
func (cl *ConcurrencyLimiter) InFlight() int {
	if cl == nil {
		return 0
	}

	return int(cl.inFlight.Load())
}

// Available returns the number of permits that can still be acquired
// immediately.
func (cl *ConcurrencyLimiter) Available() int {
	if cl == nil {
		return 0
	}

	available := cl.Limit() - cl.InFlight()
	if available < 0 {
		return 0
	}

	return available
}

// TryAcquire attempts to take a permit without blocking.
//
// This is the correct API when the caller should return immediately with a
// rejection, retryable error, or reschedule decision rather than waiting for
// capacity.
func (cl *ConcurrencyLimiter) TryAcquire() (*ConcurrencyPermit, bool) {
	if cl == nil || cl.permits == nil {
		return nil, false
	}

	select {
	case cl.permits <- struct{}{}:
		cl.inFlight.Add(1)
		return &ConcurrencyPermit{limiter: cl}, true
	default:
		return nil, false
	}
}

// Acquire waits until a permit is available or the context is cancelled.
//
// Callers should almost always provide a context with a deadline. An unbounded
// wait simply moves overload from "too much concurrent work" to "too many
// blocked goroutines waiting for work".
func (cl *ConcurrencyLimiter) Acquire(ctx context.Context) (*ConcurrencyPermit, error) {
	if cl == nil || cl.permits == nil {
		return nil, errors.New("concurrency limiter is not initialized")
	}

	select {
	case cl.permits <- struct{}{}:
		cl.inFlight.Add(1)
		return &ConcurrencyPermit{limiter: cl}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Release frees the acquired permit. It is safe to call more than once.
func (p *ConcurrencyPermit) Release() {
	if p == nil || p.limiter == nil {
		return
	}

	p.once.Do(func() {
		<-p.limiter.permits
		p.limiter.inFlight.Add(-1)
	})
}

// Execute acquires a permit, runs fn, and always releases the permit.
//
// This helper is useful when callers want one clear acquisition site and want
// to avoid forgetting a deferred Release. It is equivalent to Acquire followed
// by a deferred Release wrapped around fn.
func (cl *ConcurrencyLimiter) Execute(ctx context.Context, fn func(context.Context) error) error {
	permit, err := cl.Acquire(ctx)
	if err != nil {
		return err
	}
	defer permit.Release()

	return fn(ctx)
}
