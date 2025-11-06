## Frame Architecture Overview

Frame is a Go-based framework built on top of [go-cloud](https://github.com/google/go-cloud) that provides a cloud-agnostic way to build modern API servers. This document outlines the core architecture and design principles of the framework.

### Core Design Principles

1. **Modularity**: Each component is independent and can be used in isolation
2. **Cloud Agnosticism**: Built on go-cloud to prevent vendor lock-in
3. **Minimal Boilerplate**: Simplified setup and configuration
4. **Runtime Efficiency**: Only initialized components are loaded
5. **Extensibility**: Easy to extend and customize components

### Key Components

#### 1. Service Layer (`service.go`)
- Central orchestrator for all framework components
- Manages component lifecycle and dependencies
- Handles graceful startup and shutdown
- Configurable through options pattern

#### 2. Server Components
- **HTTP Server**
  - Built on gorilla/mux
  - Configurable middleware support
  - Health check endpoints
  - Request logging
  
- **gRPC Server**
  - Native gRPC support
  - Bi-directional streaming
  - Protocol buffer integration

#### 3. Data Layer
- **Database Management** (`datastore.go`)
  - GORM integration for ORM capabilities
  - Multi-tenancy support
  - Read/Write separation
  - Migration management
  
- **Queue System** (`queue.go`)
  - Asynchronous message processing
  - Multiple queue backend support (memory, NATS, GCP PubSub)
  - Publisher/Subscriber pattern
  - Message handling with retries

#### 4. Security Components
- **Authentication** (`authentication.go`)
  - OAuth2 support
  - JWT token handling
  - Flexible auth provider integration
  
- **Authorization** (`authorization.go`)
  - Role-based access control
  - Permission management
  - Policy enforcement

#### 5. Support Features
- **Configuration** (`config.go`)
  - Environment-based configuration
  - Secret management
  - Dynamic configuration updates

- **Logging** (`logger.go`)
  - Structured logging
  - Log level management
  - Context-aware logging

- **Tracing** (`tracing.go`)
  - Distributed tracing support
  - OpenTelemetry integration
  - Performance monitoring

### Component Interaction Flow

1. Service initialization starts with `NewService(frame.WithName())`
2. Components are registered through options
3. Service manages component lifecycle:
   - Initialization order
   - Dependency injection
   - Graceful shutdown
4. Request flow:
   - HTTP/gRPC request received
   - Authentication/Authorization
   - Request processing
   - Response handling

### Best Practices

1. **Component Initialization**
   - Initialize only required components
   - Use appropriate options for customization
   - Handle errors during initialization

2. **Error Handling**
   - Use context for cancellation
   - Implement proper error wrapping
   - Provide meaningful error messages

3. **Configuration**
   - Use environment variables for configuration
   - Implement proper validation
   - Follow secure practices for sensitive data

4. **Testing**
   - Write unit tests for components
   - Use integration tests for component interaction
   - Implement proper mocking
