package modules

import (
	"context"
	"fmt"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/internal/frameauth"
)

// AuthModule wraps the frameauth.Authenticator to implement the Module interface
type AuthModule struct {
	authenticator frameauth.Authenticator
	status        frame.ModuleStatus
	config        frameauth.Config
}

// NewAuthModule creates a new authentication module
func NewAuthModule(config frameauth.Config, logger frameauth.Logger) *AuthModule {
	authenticator := frameauth.NewAuthenticator(config, logger)
	
	return &AuthModule{
		authenticator: authenticator,
		status:        frame.ModuleStatusUnloaded,
		config:        config,
	}
}

// Type returns the module type identifier
func (m *AuthModule) Type() frame.ModuleType {
	return frame.ModuleTypeAuthentication
}

// Name returns a human-readable name for the module
func (m *AuthModule) Name() string {
	return "Authentication Module"
}

// Version returns the module version
func (m *AuthModule) Version() string {
	return "1.0.0"
}

// Status returns the current module status
func (m *AuthModule) Status() frame.ModuleStatus {
	return m.status
}

// Dependencies returns a list of module types this module depends on
func (m *AuthModule) Dependencies() []frame.ModuleType {
	return []frame.ModuleType{} // No dependencies
}

// Initialize prepares the module for use with the given configuration
func (m *AuthModule) Initialize(ctx context.Context, config any) error {
	if config != nil {
		if authConfig, ok := config.(frameauth.Config); ok {
			m.config = authConfig
		} else {
			return fmt.Errorf("invalid configuration type for authentication module")
		}
	}
	
	m.status = frame.ModuleStatusLoaded
	return nil
}

// Start begins the module's operation
func (m *AuthModule) Start(ctx context.Context) error {
	if m.status != frame.ModuleStatusLoaded {
		return fmt.Errorf("module must be loaded before starting")
	}
	
	m.status = frame.ModuleStatusStarted
	return nil
}

// Stop gracefully shuts down the module
func (m *AuthModule) Stop(ctx context.Context) error {
	if m.status != frame.ModuleStatusStarted {
		return nil // Already stopped
	}
	
	m.status = frame.ModuleStatusStopped
	return nil
}

// IsEnabled returns whether the module is currently enabled
func (m *AuthModule) IsEnabled() bool {
	return m.authenticator != nil && m.authenticator.IsEnabled()
}

// HealthCheck returns nil if the module is healthy, error otherwise
func (m *AuthModule) HealthCheck() error {
	if m.authenticator == nil {
		return fmt.Errorf("authenticator not initialized")
	}
	
	if !m.IsEnabled() {
		return fmt.Errorf("authentication module is disabled")
	}
	
	return nil
}

// Authenticator returns the underlying authenticator implementation
func (m *AuthModule) Authenticator() frameauth.Authenticator {
	return m.authenticator
}
