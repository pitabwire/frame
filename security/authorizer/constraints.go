package authorizer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pitabwire/frame/security"
)

// AccessConstraint evaluates a contextual condition that must hold for access
// to be granted. It is checked after the Keto relation check passes.
// Return nil to allow, or a non-nil error to deny.
type AccessConstraint func(ctx context.Context) error

type constraintCtxKey string

const (
	ctxKeyCurrentTime constraintCtxKey = "current_time"
	ctxKeyLocation    constraintCtxKey = "location"
)

// WithCurrentTime injects the current time into context. Production middleware
// should call this with time.Now(). Tests can inject a fixed value.
func WithCurrentTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, ctxKeyCurrentTime, t)
}

// CurrentTimeFromContext returns the time stored in context, or time.Now()
// if none was injected.
func CurrentTimeFromContext(ctx context.Context) time.Time {
	if t, ok := ctx.Value(ctxKeyCurrentTime).(time.Time); ok {
		return t
	}
	return time.Now()
}

// WithLocation injects the caller's location identifier into context.
// The value is normalized to lowercase for case-insensitive matching.
func WithLocation(ctx context.Context, location string) context.Context {
	return context.WithValue(ctx, ctxKeyLocation, strings.ToLower(location))
}

// LocationFromContext returns the location stored in context, or "".
func LocationFromContext(ctx context.Context) string {
	loc, _ := ctx.Value(ctxKeyLocation).(string)
	return loc
}

// TimeWindowConstraint restricts access to a daily time window defined by
// startHour and endHour (0-23, 24h format). Access is allowed when
// startHour <= currentHour < endHour. Supports wrapping past midnight
// (e.g., startHour=22, endHour=6 allows 22:00-05:59).
// If loc is nil, UTC is used.
//
// Panics if startHour or endHour is outside 0-23, or if startHour == endHour
// (ambiguous — use no constraint for "always allow").
func TimeWindowConstraint(startHour, endHour int, loc *time.Location) AccessConstraint {
	if startHour < 0 || startHour > 23 {
		panic(fmt.Sprintf("authorizer: TimeWindowConstraint startHour %d out of range 0-23", startHour))
	}
	if endHour < 0 || endHour > 23 {
		panic(fmt.Sprintf("authorizer: TimeWindowConstraint endHour %d out of range 0-23", endHour))
	}
	if startHour == endHour {
		panic(fmt.Sprintf("authorizer: TimeWindowConstraint startHour == endHour (%d) is ambiguous", startHour))
	}
	if loc == nil {
		loc = time.UTC
	}
	return func(ctx context.Context) error {
		now := CurrentTimeFromContext(ctx).In(loc)
		hour := now.Hour()

		var allowed bool
		if startHour < endHour {
			// Same-day window: e.g., 9-17
			allowed = hour >= startHour && hour < endHour
		} else {
			// Wraps midnight: e.g., 22-6 means 22:00-05:59
			allowed = hour >= startHour || hour < endHour
		}

		if !allowed {
			return fmt.Errorf("access denied: current hour %d is outside allowed window %02d:00-%02d:00",
				hour, startHour, endHour)
		}
		return nil
	}
}

// LocationConstraint restricts access to callers whose location (from context)
// matches one of the allowed values. Matching is case-insensitive.
// Returns an error if no location is present in context or if the location
// is not in the allowed set.
//
// Panics if no allowed locations are provided.
func LocationConstraint(allowed ...string) AccessConstraint {
	if len(allowed) == 0 {
		panic("authorizer: LocationConstraint requires at least one allowed location")
	}
	set := make(map[string]bool, len(allowed))
	for _, loc := range allowed {
		set[strings.ToLower(loc)] = true
	}
	return func(ctx context.Context) error {
		loc := LocationFromContext(ctx)
		if loc == "" {
			return errors.New("access denied: no location in request context")
		}
		if !set[loc] {
			return fmt.Errorf("access denied: location %q is not in the allowed set", loc)
		}
		return nil
	}
}

// AnyConstraint returns a constraint that passes if at least one of the given
// constraints passes (OR logic). All constraint errors are collected; if none
// pass, the error from the first constraint is returned.
//
// Panics if no constraints are provided.
func AnyConstraint(constraints ...AccessConstraint) AccessConstraint {
	if len(constraints) == 0 {
		panic("authorizer: AnyConstraint requires at least one constraint")
	}
	return func(ctx context.Context) error {
		var firstErr error
		for _, c := range constraints {
			if err := c(ctx); err == nil {
				return nil
			} else if firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

// evaluateConstraints runs a slice of constraints with panic recovery.
// Returns nil if all pass, or a PermissionDeniedError on the first failure.
// If a constraint panics, the panic is converted to a denial.
func evaluateConstraints(
	ctx context.Context,
	constraints []AccessConstraint,
	permission string,
	obj security.ObjectRef,
	sub security.SubjectRef,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = NewPermissionDeniedError(obj, permission, sub,
				fmt.Sprintf("constraint panic: %v", r))
		}
	}()

	for _, constraint := range constraints {
		if cErr := constraint(ctx); cErr != nil {
			return NewPermissionDeniedError(obj, permission, sub, cErr.Error())
		}
	}
	return nil
}

// evaluateConstraintsForPermission runs global constraints and any
// permission-specific constraints, with panic recovery.
func evaluateConstraintsForPermission(
	ctx context.Context,
	constraints []AccessConstraint,
	permConstraints map[string][]AccessConstraint,
	permission string,
	obj security.ObjectRef,
	sub security.SubjectRef,
) error {
	if err := evaluateConstraints(ctx, constraints, permission, obj, sub); err != nil {
		return err
	}
	if pc, ok := permConstraints[permission]; ok {
		return evaluateConstraints(ctx, pc, permission, obj, sub)
	}
	return nil
}
