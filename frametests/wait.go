package frametests

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/frame"
)

// WaitForConditionWithResult polls a condition function until it returns a non-nil result or timeout occurs
// This is useful when you need to wait for a specific result from an operation.
func WaitForConditionWithResult[T any](
	ctx context.Context,
	condition func() (*T, error),
	timeout time.Duration,
	pollInterval time.Duration,
) (*T, error) {
	return WaitForCheckedConditionWithResult(ctx, condition, func(t *T, err error) bool {
		if err != nil {
			if !frame.ErrorIsNoRows(err) {
				return true
			}
		} else {
			if t != nil {
				return true
			}
		}
		return false
	}, timeout, pollInterval)
}

// WaitForCheckedConditionWithResult polls a condition function until it returns a non-nil result or timeout occurs
// This is useful when you need to wait for a specific result from an operation.
func WaitForCheckedConditionWithResult[T any](
	ctx context.Context,
	condition func() (*T, error), canReturnChecker func(*T, error) bool,
	timeout time.Duration,
	pollInterval time.Duration,
) (*T, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		result, err := condition()
		if canReturnChecker(result, err) {
			return result, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	return nil, fmt.Errorf("condition not met within timeout of %v", timeout)
}
