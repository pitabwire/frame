// Package setup defines an abstract, Frame-agnostic model for one-shot
// setup work: schema migrations, permission publishing, root/bot bootstrap,
// health probes, and any other bulk steps a deploy job must run before
// traffic.
//
// Design goals:
//
//   - Steps implement a tiny interface (Name + Run) — no Service, HTTP, or
//     Cloud Run types in the contract.
//   - A Registry holds steps and executes them in bulk (all, or a named
//     subset) fail-closed and in order.
//   - Selection (which process is a setup job, which step names) is pure
//     config/argv parsing — callers wire env/CLI however they like.
//
// Frame integrates via thin adapters (frame.WithSetupStep, Service.Setup)
// so applications can also use this package outside Frame if needed.
package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Well-known step names. Applications may register any other stable names
// (e.g. "bootstrap", "verify", "seed"). Conventions only — the registry
// does not special-case these strings.
const (
	// NameMigrate is the conventional name for schema migrations.
	NameMigrate = "migrate"

	// NamePermissions is the conventional name for publishing a service
	// permission manifest (e.g. to tenancy).
	NamePermissions = "permissions"

	// NameBootstrap is the conventional name for root/bot/seed work that
	// must run once after migrate and before traffic.
	NameBootstrap = "bootstrap"

	// NameVerify is the conventional name for post-setup checks
	// (connectivity, required rows, tuple presence, etc.).
	NameVerify = "verify"
)

// Step is one idempotent unit of work in a setup plan.
//
// Contracts:
//   - Name is stable and unique within a Registry (case-sensitive).
//   - Run is fail-closed: a non-nil error aborts the plan.
//   - Run must be safe to re-execute after success (no-op or upsert).
type Step interface {
	Name() string
	Run(ctx context.Context) error
}

// Func is a Step backed by a function. Use when you do not need a custom type.
//
//	setup.Func{StepName: setup.NameMigrate, Fn: migrateFn}
type Func struct {
	StepName string
	Fn       func(ctx context.Context) error
}

// Name implements Step.
func (f Func) Name() string { return strings.TrimSpace(f.StepName) }

// Run implements Step.
func (f Func) Run(ctx context.Context) error {
	if f.Fn == nil {
		return fmt.Errorf("setup step %q: nil Fn", f.Name())
	}
	return f.Fn(ctx)
}

// ErrUnknownStep is returned when Run is asked for a name that was never registered.
var ErrUnknownStep = errors.New("unknown setup step")

// ErrEmptyPlan is returned when Run has nothing to execute.
var ErrEmptyPlan = errors.New("setup: no steps requested and none registered")
