package setup

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/pitabwire/util"
)

// Registry holds named setup steps and executes them in bulk.
//
// Registration order is preserved. Registering the same Name again replaces
// the previous Step but keeps its position in the order.
//
// Registry is safe for concurrent Register / Names; Run should be called
// from a single goroutine (typical for a Job main).
type Registry struct {
	mu     sync.Mutex
	order  []string
	byName map[string]Step
}

// NewRegistry returns an empty step registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]Step),
	}
}

// Register adds or replaces a step. Empty names and nil steps are ignored.
func (r *Registry) Register(step Step) {
	if r == nil || step == nil {
		return
	}
	name := strings.TrimSpace(step.Name())
	if name == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.byName == nil {
		r.byName = make(map[string]Step)
	}
	if _, exists := r.byName[name]; !exists {
		r.order = append(r.order, name)
	}
	r.byName[name] = step
}

// RegisterFunc is sugar for Register(Func{StepName: name, Fn: fn}).
func (r *Registry) RegisterFunc(name string, fn func(ctx context.Context) error) {
	r.Register(Func{StepName: name, Fn: fn})
}

// Names returns registered step names in registration order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Get returns a registered step by name.
func (r *Registry) Get(name string) (Step, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byName[strings.TrimSpace(name)]
	return s, ok
}

// Len returns how many steps are registered.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.order)
}

// Run executes steps fail-closed, in the order of names.
//
//   - If names is non-empty, those names run in the given order.
//   - If names is empty, every registered step runs in registration order.
//
// Each step is logged at Info on start/success. The first error aborts the
// plan and is wrapped with the step name.
func (r *Registry) Run(ctx context.Context, names ...string) error {
	if r == nil {
		return ErrEmptyPlan
	}

	selected := filterEmpty(names)
	if len(selected) == 0 {
		selected = r.Names()
	}
	if len(selected) == 0 {
		return ErrEmptyPlan
	}

	log := util.Log(ctx)
	log.Info("setup plan starting", "steps", selected)

	for _, name := range selected {
		step, ok := r.Get(name)
		if !ok {
			return fmt.Errorf("%w: %q (registered=%v)", ErrUnknownStep, name, r.Names())
		}
		log.Info("setup step running", "step", name)
		if err := step.Run(ctx); err != nil {
			return fmt.Errorf("setup step %q: %w", name, err)
		}
		log.Info("setup step complete", "step", name)
	}

	log.Info("setup plan finished", "steps", selected)
	return nil
}

// RunAll is Run with no name filter (every registered step, registration order).
func (r *Registry) RunAll(ctx context.Context) error {
	return r.Run(ctx)
}

func filterEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
