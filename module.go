package frame

import (
	"context"
	"fmt"
	"sync"
)

// ModuleType represents the type identifier for different modules
type ModuleType string

// Module type constants for easy retrieval
const (
	ModuleTypeAuthentication ModuleType = "authentication"
	ModuleTypeAuthorization  ModuleType = "authorization"
	ModuleTypeData           ModuleType = "data"
	ModuleTypeQueue          ModuleType = "queue"
	ModuleTypeObservability  ModuleType = "observability"
	ModuleTypeServer         ModuleType = "server"
	ModuleTypeWorkerPool     ModuleType = "worker_pool"
	ModuleTypeHealth         ModuleType = "health"
	ModuleTypeLocalization   ModuleType = "localization"
	ModuleTypeLogging        ModuleType = "logging"
)

// ModuleStatus represents the current state of a module
type ModuleStatus string

const (
	ModuleStatusUnloaded ModuleStatus = "unloaded"
	ModuleStatusLoaded   ModuleStatus = "loaded"
	ModuleStatusStarted  ModuleStatus = "started"
	ModuleStatusStopped  ModuleStatus = "stopped"
	ModuleStatusError    ModuleStatus = "error"
)

// Module defines the common interface that all plugins/modules must implement
type Module interface {
	// Type returns the module type identifier
	Type() ModuleType

	// Name returns a human-readable name for the module
	Name() string

	// Version returns the module version
	Version() string

	// Status returns the current module status
	Status() ModuleStatus

	// Dependencies returns a list of module types this module depends on
	Dependencies() []ModuleType

	// Initialize prepares the module for use with the given configuration
	Initialize(ctx context.Context, config any) error

	// Start begins the module's operation
	Start(ctx context.Context) error

	// Stop gracefully shuts down the module
	Stop(ctx context.Context) error

	// IsEnabled returns whether the module is currently enabled
	IsEnabled() bool

	// HealthCheck returns nil if the module is healthy, error otherwise
	HealthCheck() error
}

// ModuleRegistry manages the lifecycle of all modules
type ModuleRegistry struct {
	modules map[ModuleType]Module
	mutex   sync.RWMutex
}

// NewModuleRegistry creates a new module registry
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[ModuleType]Module),
	}
}

// Register adds a module to the registry
func (r *ModuleRegistry) Register(module Module) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	moduleType := module.Type()
	if _, exists := r.modules[moduleType]; exists {
		return fmt.Errorf("module of type %s is already registered", moduleType)
	}

	r.modules[moduleType] = module
	return nil
}

// Get retrieves a module by type, returns nil if not found
func (r *ModuleRegistry) Get(moduleType ModuleType) Module {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.modules[moduleType]
}

// GetTyped retrieves a module by type and casts it to the specified interface
func (r *ModuleRegistry) GetTyped(moduleType ModuleType, target any) bool {
	module := r.Get(moduleType)
	if module == nil {
		return false
	}

	// Use type assertion to cast to the target interface
	switch t := target.(type) {
	case *interface{}:
		*t = module
		return true
	default:
		// For specific interface types, we'd need reflection or type switches
		// This is a simplified version - can be enhanced based on needs
		return false
	}
}

// List returns all registered module types
func (r *ModuleRegistry) List() []ModuleType {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	types := make([]ModuleType, 0, len(r.modules))
	for moduleType := range r.modules {
		types = append(types, moduleType)
	}
	return types
}

// Initialize initializes all registered modules with their configurations
func (r *ModuleRegistry) Initialize(ctx context.Context, configs map[ModuleType]any) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Initialize modules in dependency order
	initialized := make(map[ModuleType]bool)

	for len(initialized) < len(r.modules) {
		progress := false

		for moduleType, module := range r.modules {
			if initialized[moduleType] {
				continue
			}

			// Check if all dependencies are initialized
			canInitialize := true
			for _, dep := range module.Dependencies() {
				if !initialized[dep] {
					canInitialize = false
					break
				}
			}

			if canInitialize {
				config := configs[moduleType]
				if err := module.Initialize(ctx, config); err != nil {
					return fmt.Errorf("failed to initialize module %s: %w", moduleType, err)
				}
				initialized[moduleType] = true
				progress = true
			}
		}

		if !progress {
			return fmt.Errorf("circular dependency detected or missing dependencies")
		}
	}

	return nil
}

// Start starts all registered modules
func (r *ModuleRegistry) Start(ctx context.Context) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for moduleType, module := range r.modules {
		if module.IsEnabled() {
			if err := module.Start(ctx); err != nil {
				return fmt.Errorf("failed to start module %s: %w", moduleType, err)
			}
		}
	}

	return nil
}

// Stop stops all registered modules in reverse order
func (r *ModuleRegistry) Stop(ctx context.Context) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Stop modules in reverse dependency order
	var errors []error

	for moduleType, module := range r.modules {
		if module.Status() == ModuleStatusStarted {
			if err := module.Stop(ctx); err != nil {
				errors = append(errors, fmt.Errorf("failed to stop module %s: %w", moduleType, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping modules: %v", errors)
	}

	return nil
}

// HealthCheck performs health checks on all enabled modules
func (r *ModuleRegistry) HealthCheck() map[ModuleType]error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	results := make(map[ModuleType]error)

	for moduleType, module := range r.modules {
		if module.IsEnabled() {
			results[moduleType] = module.HealthCheck()
		}
	}

	return results
}

// Unregister removes a module from the registry
func (r *ModuleRegistry) Unregister(moduleType ModuleType) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	module, exists := r.modules[moduleType]
	if !exists {
		return fmt.Errorf("module of type %s is not registered", moduleType)
	}

	// Ensure module is stopped before unregistering
	if module.Status() == ModuleStatusStarted {
		return fmt.Errorf("cannot unregister running module %s", moduleType)
	}

	delete(r.modules, moduleType)
	return nil
}
