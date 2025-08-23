# Frame Plugin Registry API Documentation

## Overview

The Frame service has been redesigned to use a **plugin/module registry pattern** that provides a clean, extensible architecture where the service doesn't need to know about specific modules beforehand. This document provides comprehensive API documentation and migration guidance.

## Core Architecture

### Module Interface

All plugins must implement the `Module` interface:

```go
type Module interface {
    // Module identification
    Type() ModuleType
    Name() string
    Version() string
    
    // Module lifecycle
    Initialize(ctx context.Context, config any) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    
    // Module status and health
    Status() ModuleStatus
    IsEnabled() bool
    HealthCheck() error
    
    // Module dependencies
    Dependencies() []ModuleType
}
```

### Module Types

Predefined module type constants for easy retrieval:

```go
const (
    ModuleTypeAuthentication ModuleType = "authentication"
    ModuleTypeAuthorization  ModuleType = "authorization"
    ModuleTypeData          ModuleType = "data"
    ModuleTypeQueue         ModuleType = "queue"
    ModuleTypeObservability ModuleType = "observability"
    ModuleTypeServer        ModuleType = "server"
)
```

### Module Status

Module lifecycle status tracking:

```go
const (
    ModuleStatusUnloaded ModuleStatus = "unloaded"
    ModuleStatusLoaded   ModuleStatus = "loaded"
    ModuleStatusStarted  ModuleStatus = "started"
    ModuleStatusStopped  ModuleStatus = "stopped"
    ModuleStatusError    ModuleStatus = "error"
)
```

## Service Interface

The new Service interface is composed of three main parts:

### CoreService

Essential service functionality always available:

```go
type CoreService interface {
    // Core service information
    Name() string
    Version() string
    Environment() string
    
    // Configuration and logging
    Config() any
    Log(ctx context.Context) *util.LogEntry
    
    // Service lifecycle management
    Init(ctx context.Context, opts ...Option)
    Run(ctx context.Context, address string) error
    Stop(ctx context.Context)
    
    // Extensibility hooks
    AddPreStartMethod(f func(ctx context.Context, s Service))
    AddCleanupMethod(f func(ctx context.Context))
}
```

### ModuleService

Module registry functionality:

```go
type ModuleService interface {
    // Module registry access
    Modules() *ModuleRegistry
    
    // Module retrieval by type
    GetModule(moduleType ModuleType) Module
    
    // Typed module retrieval with interface casting
    GetTypedModule(moduleType ModuleType, target any) bool
    
    // Register a new module
    RegisterModule(module Module) error
    
    // Check if a module is available and enabled
    HasModule(moduleType ModuleType) bool
}
```

### LegacyService

Backward compatibility for existing functionality:

```go
type LegacyService interface {
    // JWT client management
    JwtClient() map[string]any
    SetJwtClient(jwtCli map[string]any)
    JwtClientID() string
    JwtClientSecret() string
    
    // HTTP handler access
    H() http.Handler
    
    // Health checking
    HandleHealth(w http.ResponseWriter, r *http.Request)
    AddHealthCheck(checker Checker)
    
    // REST service invocation
    InvokeRestService(ctx context.Context, method string, endpointURL string, payload map[string]any, headers map[string][]string) (int, []byte, error)
    InvokeRestServiceURLEncoded(ctx context.Context, method string, endpointURL string, payload url.Values, headers map[string]string) (int, []byte, error)
}
```

## Module Registry

The `ModuleRegistry` manages module lifecycle with thread-safe operations:

```go
type ModuleRegistry struct {
    // Thread-safe module management
    Register(module Module) error
    Get(moduleType ModuleType) Module
    GetTyped(moduleType ModuleType, target any) bool
    List() []ModuleType
    
    // Lifecycle management
    Initialize(ctx context.Context) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    
    // Health checking
    HealthCheck() error
}
```

## Helper Functions

Type-safe module retrieval functions for end users:

```go
// Authentication module
func GetAuthenticator(service Service) frameauth.Authenticator

// Authorization module  
func GetAuthorizer(service Service) frameauthorization.Authorizer

// Data modules
func GetDatastoreManager(service Service) framedata.DatastoreManager
func GetMigrator(service Service) framedata.Migrator
func GetSearchProvider(service Service) framedata.SearchProvider

// Queue module
func GetQueueManager(service Service) framequeue.QueueManager

// Observability module
func GetObservabilityManager(service Service) frameobservability.ObservabilityManager

// Server module
func GetServerManager(service Service) frameserver.ServerManager

// Utility functions
func IsModuleEnabled(service Service, moduleType ModuleType) bool
func ListEnabledModules(service Service) []ModuleType
```

## Usage Examples

### Basic Service Creation

```go
// Create service with auto-configuration
ctx, service := frame.NewService("my-service", 
    frame.WithAutoConfiguration(),
)

// Run the service
err := service.Run(ctx, ":8080")
```

### Manual Module Registration

```go
// Create and register custom modules
authModule := frame.NewAuthModule(authenticator)
err := service.RegisterModule(authModule)

dataModule := frame.NewDataModule(datastoreManager, migrator)
err = service.RegisterModule(dataModule)
```

### Module Retrieval

```go
// Type-safe module retrieval
authenticator := frame.GetAuthenticator(service)
if authenticator != nil {
    // Use authenticator
}

// Direct module access
authModule := service.GetModule(frame.ModuleTypeAuthentication)
if authModule != nil && authModule.IsEnabled() {
    // Use module
}

// Check if module is available
if service.HasModule(frame.ModuleTypeData) {
    datastoreManager := frame.GetDatastoreManager(service)
    // Use datastore manager
}
```

### Module Status Inspection

```go
// List all enabled modules
enabledModules := frame.ListEnabledModules(service)
for _, moduleType := range enabledModules {
    module := service.GetModule(moduleType)
    fmt.Printf("Module %s: %s (v%s)\n", 
        module.Type(), module.Name(), module.Version())
}

// Check module health
registry := service.Modules()
err := registry.HealthCheck()
if err != nil {
    // Handle unhealthy modules
}
```

## Migration Guide

### From Old Service Interface

**Before (Old Pattern):**
```go
// Direct module access
authenticator := service.Authenticator()
datastoreManager := service.DatastoreManager()
```

**After (New Plugin Registry):**
```go
// Module registry access
authenticator := frame.GetAuthenticator(service)
datastoreManager := frame.GetDatastoreManager(service)
```

### Module Implementation

**Before (Direct Integration):**
```go
// Modules were directly embedded in service
type serviceImpl struct {
    authenticator frameauth.Authenticator
    datastoreManager framedata.DatastoreManager
}
```

**After (Registry Pattern):**
```go
// Modules are managed by registry
type serviceImpl struct {
    moduleRegistry *ModuleRegistry
}

// Modules implement common interface
type AuthModule struct {
    authenticator frameauth.Authenticator
}

func (m *AuthModule) Type() ModuleType { return ModuleTypeAuthentication }
func (m *AuthModule) Initialize(ctx context.Context, config any) error { /* ... */ }
// ... other Module interface methods
```

### Configuration Options

**Before:**
```go
// Direct module configuration
service.SetAuthenticator(authenticator)
service.SetDatastoreManager(datastoreManager)
```

**After:**
```go
// Module registration through options
frame.WithAuthentication()(ctx, service)
frame.WithDataModule()(ctx, service)

// Or manual registration
authModule := frame.NewAuthModule(authenticator)
service.RegisterModule(authModule)
```

## Benefits

1. **Dynamic Module Loading**: Service doesn't need to know about specific modules beforehand
2. **Clean Extensibility**: New modules can be added without changing core service code
3. **Registry-Based Access**: Modules retrieved via constants/identifiers
4. **Lifecycle Management**: Proper initialization, startup, and shutdown ordering
5. **Health Monitoring**: Built-in health checking for all modules
6. **Dependency Management**: Automatic dependency resolution and ordering
7. **Backward Compatibility**: Existing APIs preserved through legacy interface

## Thread Safety

The module registry is designed to be thread-safe:

- All registry operations use appropriate locking mechanisms
- Module lifecycle operations are synchronized
- Concurrent access to modules is safe after initialization

## Error Handling

The plugin registry provides comprehensive error handling:

- Module registration failures are reported with detailed errors
- Module lifecycle errors are captured and can be inspected
- Health check failures are tracked per module
- Dependency resolution errors are clearly reported

This plugin registry pattern provides a robust, extensible foundation for the Frame service architecture while maintaining backward compatibility with existing code.
