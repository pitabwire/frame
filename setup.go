package frame

import (
	"context"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/setup"
)

// Re-export well-known setup step names so application code can stay on the
// frame import path when preferred. The abstract API lives in package setup.
const (
	SetupTaskMigrate     = setup.NameMigrate
	SetupTaskPermissions = setup.NamePermissions
	SetupTaskBootstrap   = setup.NameBootstrap
	SetupTaskVerify      = setup.NameVerify
)

// ErrUnknownSetupTask is setup.ErrUnknownStep (stable alias for callers).
var ErrUnknownSetupTask = setup.ErrUnknownStep

// SetupFunc is a setup step that closes over *Service (migrate with datastore,
// permission publish with HTTP client, etc.). Prefer setup.Step / setup.Func
// when the step does not need the Service.
type SetupFunc func(ctx context.Context, s *Service) error

// Setup returns the service's setup.Registry (lazy-initialized).
// Applications register abstract steps here and call RunSetup / Registry.Run
// from a one-shot Job. Runtime servers typically never call Run.
func (s *Service) Setup() *setup.Registry {
	if s == nil {
		return setup.NewRegistry()
	}
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	if s.setupRegistry == nil {
		s.setupRegistry = setup.NewRegistry()
	}
	return s.setupRegistry
}

// WithSetupStep registers an abstract setup.Step on the service registry.
func WithSetupStep(step setup.Step) Option {
	return func(_ context.Context, s *Service) {
		s.Setup().Register(step)
	}
}

// WithSetupFunc registers a name + function that does not need *Service.
func WithSetupFunc(name string, fn func(ctx context.Context) error) Option {
	return WithSetupStep(setup.Func{StepName: name, Fn: fn})
}

// WithSetupTask registers a Service-aware step (closes over s at Run time).
// Prefer WithSetupStep / setup.Func when possible.
func WithSetupTask(name string, fn SetupFunc) Option {
	return func(_ context.Context, s *Service) {
		s.AddSetupTask(name, fn)
	}
}

// AddSetupTask registers a Service-aware step on the setup registry.
func (s *Service) AddSetupTask(name string, fn SetupFunc) {
	if s == nil || fn == nil {
		return
	}
	svc := s
	s.Setup().RegisterFunc(name, func(ctx context.Context) error {
		return fn(ctx, svc)
	})
}

// SetupTaskNames returns registered setup step names in registration order.
func (s *Service) SetupTaskNames() []string {
	if s == nil {
		return nil
	}
	return s.Setup().Names()
}

// RunSetup executes the setup plan fail-closed via setup.Registry.Run.
//
// Task selection:
//   - explicit taskNames if provided
//   - else names from SetupSelection(cfg) / argv / FRAME_SETUP_TASKS
//   - else every registered step (empty name list)
//
// Typical Cloud Run Job:
//
//	args: ["setup", "migrate", "permissions", "bootstrap"]
//	if frame.IsSetupMode(cfg) { return svc.RunSetup(ctx) }
func (s *Service) RunSetup(ctx context.Context, taskNames ...string) error {
	if s == nil {
		return setup.ErrEmptyPlan
	}
	names := taskNames
	if len(names) == 0 {
		names = SetupSelection(s.Config()).Names
	}
	return s.Setup().Run(ctx, names...)
}

// IsSetupMode reports whether this process should run setup and exit instead
// of serving traffic. See setup.Selection / config.ConfigurationSetup.
func IsSetupMode(cfg any) bool {
	return SetupSelection(cfg).Active
}

// SetupTasksFromConfig returns the step name list from config/argv.
// Empty means “all registered steps”.
func SetupTasksFromConfig(cfg any) []string {
	return SetupSelection(cfg).Names
}

// SetupSelection returns setup.Selection for cfg (Active + Names).
// Prefer this when you want to branch on Active and pass Names to Registry.Run.
func SetupSelection(cfg any) setup.Selection {
	if c, ok := cfg.(config.ConfigurationSetup); ok {
		// IsSetupMode / GetSetupTasks already fold argv + DO_SETUP + CSV.
		return setup.Selection{
			Active: c.IsSetupMode(),
			Names:  c.GetSetupTasks(),
		}
	}
	return setup.SelectFromOS(false, "")
}
