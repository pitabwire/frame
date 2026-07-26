package frame

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v2/config"
)

// Well-known setup task names. Apps may register additional names (e.g. "bootstrap").
const (
	// SetupTaskMigrate is the conventional name for schema migrations.
	// Frame does not run SQL itself — services register this task with their
	// repository.Migrate (or equivalent) implementation.
	SetupTaskMigrate = "migrate"

	// SetupTaskPermissions publishes the service permission manifest to
	// tenancy (see WithPermissionRegistration). Registered automatically
	// when PERMISSIONS_REGISTRATION_URL is set.
	SetupTaskPermissions = "permissions"
)

// SetupFunc is a one-shot step run by a Cloud Run / Helm setup job.
// It must be idempotent: re-running setup after success is a no-op or upsert.
type SetupFunc func(ctx context.Context, s *Service) error

type setupTask struct {
	name string
	run  SetupFunc
}

// ErrUnknownSetupTask is returned when RunSetup is asked for a task that
// was never registered with AddSetupTask / WithSetupTask.
var ErrUnknownSetupTask = errors.New("unknown setup task")

// WithSetupTask registers a named setup step for RunSetup.
// Prefer stable names (SetupTaskMigrate, "bootstrap", …).
// Later registrations with the same name replace the earlier function.
func WithSetupTask(name string, fn SetupFunc) Option {
	return func(_ context.Context, s *Service) {
		s.AddSetupTask(name, fn)
	}
}

// AddSetupTask registers (or replaces) a named setup step on the service.
func (s *Service) AddSetupTask(name string, fn SetupFunc) {
	name = strings.TrimSpace(name)
	if name == "" || fn == nil {
		return
	}
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()

	for i := range s.setupTasks {
		if s.setupTasks[i].name == name {
			s.setupTasks[i].run = fn
			return
		}
	}
	s.setupTasks = append(s.setupTasks, setupTask{name: name, run: fn})
}

// SetupTaskNames returns registered setup task names in registration order.
func (s *Service) SetupTaskNames() []string {
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	out := make([]string, len(s.setupTasks))
	for i, t := range s.setupTasks {
		out[i] = t.name
	}
	return out
}

// RunSetup executes setup tasks in order and returns when all succeed.
//
// Task selection:
//   - If taskNames is non-empty, those names run in the given order.
//   - Else if config/env/args yield a list (SetupTasks / FRAME_SETUP_TASKS /
//     `setup a b c`), that list is used.
//   - Else every registered task runs in registration order.
//
// Intended for Cloud Run Jobs / Helm pre-upgrade:
//
//	args: ["setup", "migrate", "permissions", "bootstrap"]
//
// Runtime replicas should not call RunSetup; they call svc.Run as usual.
// Prefer PERMISSIONS_REGISTER_ON_START=false on those replicas when
// permissions are published from the setup job.
func (s *Service) RunSetup(ctx context.Context, taskNames ...string) error {
	tasks := taskNames
	if len(tasks) == 0 {
		tasks = resolveSetupTaskList(s.Config())
	}
	if len(tasks) == 0 {
		s.startupRegistrations.Lock()
		for _, t := range s.setupTasks {
			tasks = append(tasks, t.name)
		}
		s.startupRegistrations.Unlock()
	}
	if len(tasks) == 0 {
		return errors.New("setup: no tasks requested and none registered")
	}

	log := util.Log(ctx)
	log.Info("setup job starting", "tasks", tasks)

	for _, name := range tasks {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		fn := s.lookupSetupTask(name)
		if fn == nil {
			return fmt.Errorf("%w: %q (registered=%v)", ErrUnknownSetupTask, name, s.SetupTaskNames())
		}
		log.Info("setup task running", "task", name)
		if err := fn(ctx, s); err != nil {
			return fmt.Errorf("setup task %q: %w", name, err)
		}
		log.Info("setup task complete", "task", name)
	}

	log.Info("setup job finished")
	return nil
}

func (s *Service) lookupSetupTask(name string) SetupFunc {
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	for _, t := range s.setupTasks {
		if t.name == name {
			return t.run
		}
	}
	return nil
}

// IsSetupMode reports whether this process should run one-shot setup and exit
// (instead of serving traffic). True when:
//   - argv is `setup …` or `setup`, or
//   - DO_SETUP / FRAME_DO_SETUP is true, or
//   - FRAME_SETUP_TASKS is non-empty (implies setup intent).
//
// The legacy `migrate` argv alone is NOT setup mode — apps keep their early
// migrate-and-exit paths for backwards compatibility. Prefer `setup migrate`
// for the unified job.
func IsSetupMode(cfg any) bool {
	if c, ok := cfg.(config.ConfigurationSetup); ok {
		return c.IsSetupMode()
	}
	return setupModeFromEnvAndArgs(false, "")
}

// SetupTasksFromConfig returns the task list from config / FRAME_SETUP_TASKS /
// argv after `setup`. Empty means “run all registered tasks”.
func SetupTasksFromConfig(cfg any) []string {
	return resolveSetupTaskList(cfg)
}

func resolveSetupTaskList(cfg any) []string {
	if c, ok := cfg.(config.ConfigurationSetup); ok {
		return c.GetSetupTasks()
	}
	return setupTasksFromEnvAndArgs("")
}

func setupModeFromEnvAndArgs(doSetup bool, setupTasksCSV string) bool {
	if doSetup {
		return true
	}
	if strings.TrimSpace(setupTasksCSV) != "" {
		return true
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "setup" {
		return true
	}
	return false
}

func setupTasksFromEnvAndArgs(setupTasksCSV string) []string {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "setup" {
		if len(args) > 1 {
			return filterEmpty(args[1:])
		}
		// bare `setup` → empty list means all registered
		return nil
	}
	return splitCSV(setupTasksCSV)
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	return filterEmpty(parts)
}

func filterEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
