package queue

import (
	"sync/atomic"
	"time"
)

// subscriberMetrics tracks operational metrics for a subscriber.
type subscriberMetrics struct {
	ActiveMessages *atomic.Int64 // Currently active messages being processed
	LastActivity   *atomic.Int64 // Last activity timestamp in UnixNano
	ProcessingTime *atomic.Int64 // Total processing time in nanoseconds
	MessageCount   *atomic.Int64 // Total messages processed
	ErrorCount     *atomic.Int64 // Total number of errors encountered
}

// IsIdle and is in waiting state.
func (m *subscriberMetrics) IsIdle(state SubscriberState) bool {
	return state == SubscriberStateWaiting && m.ActiveMessages.Load() <= 0
}

// IdleTime returns the duration since last activity if the subscriber is idle.
func (m *subscriberMetrics) IdleTime(state SubscriberState) time.Duration {
	if !m.IsIdle(state) {
		return 0
	}

	lastActivity := m.LastActivity.Load()
	if lastActivity == 0 {
		return 0
	}

	return time.Since(time.Unix(0, lastActivity))
}

// AverageProcessingTime returns the average time spent processing messages.
func (m *subscriberMetrics) AverageProcessingTime() time.Duration {
	count := m.MessageCount.Load()
	if count == 0 {
		return 0
	}

	return time.Duration(m.ProcessingTime.Load() / count)
}

func (m *subscriberMetrics) closeMessage(startTime time.Time, err error) {
	if err != nil {
		m.ErrorCount.Add(1)
	}

	// Update metrics after processing
	m.ProcessingTime.Add(time.Since(startTime).Nanoseconds())
	m.MessageCount.Add(1)
	m.ActiveMessages.Add(-1)
	m.LastActivity.Store(time.Now().UnixNano())
}
