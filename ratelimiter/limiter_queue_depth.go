package ratelimiter

import (
	"context"
	"errors"
	"sync"
	"time"
)

// QueueDepthFunc reports the current backlog depth of the protected queue or
// work source.
//
// The reported depth must be the backlog whose growth actually matters for the
// admission decision at the call site. For example, if an HTTP ingest handler
// is trying to protect an execution worker queue, the depth function should
// report the execution worker backlog, not an unrelated socket buffer or a
// different queue.
type QueueDepthFunc func(context.Context) (int64, error)

// QueueDepthConfig defines a queue-depth admission controller.
//
// RejectAtDepth is the depth at which new requests start being rejected.
//
// ResumeAtDepth is the depth below which admission resumes after the limiter
// has entered reject mode. ResumeAtDepth must be strictly lower than
// RejectAtDepth to provide hysteresis and avoid flapping around a single
// threshold.
//
// RefreshInterval controls how often the queue depth source is queried. The
// depth function is commonly backed by Redis, JetStream, SQL, or another
// external system. Polling that dependency on every request would turn the
// limiter into a new bottleneck, so the limiter caches the last observation for
// this interval.
//
// FailOpen decides what happens when backlog cannot be measured:
//
//   - true: allow work to continue on measurement failure
//   - false: reject work when backlog cannot be measured
//
// FailOpen is appropriate when temporary blindness is less dangerous than
// dropping work. FailOpen=false is appropriate when protected systems are so
// sensitive that inability to measure backlog must conservatively stop
// admission.
type QueueDepthConfig struct {
	RejectAtDepth int64
	ResumeAtDepth int64

	RefreshInterval time.Duration
	FailOpen        bool
}

// QueueDepthState is a snapshot of the limiter's current observation and mode.
//
// Rejecting reflects the state after hysteresis is applied. It is therefore not
// equivalent to simply checking whether Depth is currently above
// RejectAtDepth.
type QueueDepthState struct {
	Depth       int64
	Rejecting   bool
	LastUpdated time.Time
}

// QueueDepthLimiter rejects new work when downstream backlog is unsafe.
//
// This limiter is an admission controller, not a traffic shaper. It does not
// smooth requests over time, refill tokens, or grant burst budgets. It answers
// one question only:
//
//   - "Given the current backlog, should this caller admit more work?"
//
// The intended use is producer-side protection when queue backlog is the
// dominant overload signal. Typical examples include:
//
//   - event ingest rejecting new events while outbox or worker backlog is too high
//   - API producers pausing enqueue while downstream processing is unhealthy
//   - scheduler loops refusing to enqueue more work while the work queue is saturated
//
// Do not use QueueDepthLimiter as a substitute for tenant fairness or request
// quotas. A nearly empty queue does not mean one noisy tenant should be allowed
// to consume all ingress capacity.
type QueueDepthLimiter struct {
	getDepth QueueDepthFunc
	config   QueueDepthConfig

	mu    sync.Mutex
	state QueueDepthState
}

// NewQueueDepthLimiter creates a backlog-based admission controller.
//
// The configuration must define a real hysteresis band: ResumeAtDepth must be
// lower than RejectAtDepth. If both thresholds are equal, the limiter would
// flap between admit and reject around a single depth value and would be
// operationally noisy.
func NewQueueDepthLimiter(getDepth QueueDepthFunc, cfg QueueDepthConfig) (*QueueDepthLimiter, error) {
	if getDepth == nil {
		return nil, errors.New("queue depth function is required")
	}
	if cfg.RejectAtDepth <= 0 {
		return nil, errors.New("reject depth must be greater than zero")
	}
	if cfg.ResumeAtDepth < 0 {
		return nil, errors.New("resume depth must be zero or greater")
	}
	if cfg.ResumeAtDepth >= cfg.RejectAtDepth {
		return nil, errors.New("resume depth must be lower than reject depth")
	}
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = time.Second
	}

	return &QueueDepthLimiter{
		getDepth: getDepth,
		config:   cfg,
	}, nil
}

// Allow reports whether new work should be admitted.
//
// The limiter refreshes queue depth at most once per RefreshInterval and reuses
// the cached observation between refreshes. Once rejection starts, the limiter
// stays in reject mode until the observed depth falls to or below
// ResumeAtDepth. This hysteresis is deliberate. Without it, a queue oscillating
// around the reject threshold would flap between allow and reject on nearly
// every request.
func (ql *QueueDepthLimiter) Allow(ctx context.Context) bool {
	if ql == nil {
		return true
	}

	state, err := ql.State(ctx)
	if err != nil {
		return ql.config.FailOpen
	}

	return !state.Rejecting
}

// State returns the current queue-depth snapshot, refreshing it when the cached
// observation is stale.
//
// This method is useful for observability and operator endpoints because it
// exposes the last observed depth, whether admission is currently closed, and
// when that observation was taken.
func (ql *QueueDepthLimiter) State(ctx context.Context) (QueueDepthState, error) {
	if ql == nil {
		return QueueDepthState{}, nil
	}

	ql.mu.Lock()
	defer ql.mu.Unlock()

	if time.Since(ql.state.LastUpdated) < ql.config.RefreshInterval && !ql.state.LastUpdated.IsZero() {
		return ql.state, nil
	}

	depth, err := ql.getDepth(ctx)
	if err != nil {
		return ql.state, err
	}

	rejecting := ql.state.Rejecting
	switch {
	case rejecting && depth <= ql.config.ResumeAtDepth:
		rejecting = false
	case !rejecting && depth >= ql.config.RejectAtDepth:
		rejecting = true
	}

	ql.state = QueueDepthState{
		Depth:       depth,
		Rejecting:   rejecting,
		LastUpdated: time.Now(),
	}

	return ql.state, nil
}
