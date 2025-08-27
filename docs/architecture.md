# Pluggable Architecture Plan

This document outlines the architectural plan for the `frame` project, focusing on creating a highly pluggable and extensible system.

## 1. Core Concepts

The architecture is based on three core concepts:

*   **Service:** The central component of the framework. It acts as a registry for modules and provides core services like configuration, logging, and lifecycle management.
*   **Module:** A self-contained unit of functionality that can be plugged into the service. Each module implements a specific interface and is responsible for a distinct domain (e.g., authentication, data access, message queuing).
*   **Option:** A function that configures the service. Options are used to enable, disable, and configure modules, as well as to set up other service-level parameters.

## 2. Directory Structure

To ensure a clean and maintainable codebase, we will adopt the following directory structure:

```
/
├── cmd/           # Entry points for different services
├── pkg/           # Shared packages and libraries
├── internal/      # Private application and library code
│   ├── core/      # Core interfaces and types (e.g., Module, Service)
│   └── modules/   # Implementations of the various modules
├── configs/       # Configuration files
├── deployments/   # Deployment manifests (e.g., Dockerfiles, Kubernetes manifests)
├── scripts/       # Scripts for automation (e.g., build, test, deploy)
└── examples/      # Example usage of the framework
```

## 3. Module Design

Each module will be designed with the following principles in mind:

*   **Interface-based:** Each module will expose its functionality through a public interface. This decouples the module's implementation from its consumers.
*   **Self-contained:** Modules should be as self-contained as possible, with minimal dependencies on other modules.
*   **Configurable:** Each module will have its own configuration struct, which can be populated from a configuration file or environment variables.
*   **Optional:** Modules should be optional and only included in the final program if they are explicitly enabled via an `Option`.

### Module Structure

Each module will have the following internal structure:

```
modules/
└── mymodule/
    ├── interface.go  # Public interface for the module
    ├── module.go     # Implementation of the module interface
    ├── config.go     # Configuration struct for the module
    └── options.go    # Option functions for enabling and configuring the module
```

## 4. Service Design

The `Service` will be responsible for the following:

*   **Module Registry:** The service will maintain a registry of all available modules.
*   **Lifecycle Management:** The service will manage the lifecycle of modules, including initialization, starting, and stopping.
*   **Dependency Injection:** The service will provide modules with their dependencies, such as a logger and a configuration object.

### Service Initialization

The service will be initialized using the `NewService` function, which accepts a list of `Option` functions. These options will be used to configure the service and its modules.

```go
// Create a new service with the authentication and data modules enabled
svc := frame.NewService(
    frame.WithAuthentication(authConfig),
    frame.WithData(dataConfig),
)
```

## 5. Example Workflow

Here's an example workflow of how a user would use the framework:

1.  **Define Configuration:** The user defines the configuration for their service in a `config.yaml` file.
2.  **Create a Service:** The user creates a new service using `frame.NewService()` and passes in the desired `Option` functions to enable and configure the modules they need.
3.  **Run the Service:** The user runs the service, which initializes and starts all the enabled modules.
4.  **Access Module Functionality:** The user can then access the functionality of the enabled modules through the `Service`'s module registry.

This pluggable architecture will make it easy to add new functionality to the framework without modifying the core service. It will also allow users to create lightweight services that only include the modules they need.