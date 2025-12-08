# Profiler Package

The profiler package provides a clean interface for managing Go's pprof server within the frame framework.

## Features

- Automatic pprof server lifecycle management
- Configuration-based enable/disable
- Configurable port binding
- Graceful shutdown support
- Thread-safe operations

## Usage

### Basic Usage

```go
import (
    "context"
    "github.com/pitabwire/frame/profiler"
    "github.com/pitabwire/frame/config"
)

func main() {
    cfg := &config.ConfigurationDefault{
        ProfilerEnable:   true,
        ProfilerPortAddr: ":6060",
    }
    
    server := profiler.NewServer()
    ctx := context.Background()
    
    // Start the profiler if enabled
    err := server.StartIfEnabled(ctx, cfg)
    if err != nil {
        panic(err)
    }
    
    // ... your application logic ...
    
    // Gracefully stop the profiler
    err = server.Stop(ctx)
    if err != nil {
        panic(err)
    }
}
```

### Integration with Frame Service

The profiler package is automatically integrated into the frame service when using the default configuration:

```go
import "github.com/pitabwire/frame"

func main() {
    // Set PROFILER_ENABLE=true environment variable
    ctx, svc := frame.NewService()
    
    // Profiler will automatically start on :6060
    err := svc.Run(ctx, "")
    if err != nil {
        panic(err)
    }
}
```

## Configuration

The profiler responds to the following configuration options:

### Environment Variables

- `PROFILER_ENABLE`: Set to "true" to enable the profiler (default: false)
- `PROFILER_PORT`: Port to bind the profiler server (default: ":6060")

### YAML Configuration

```yaml
profiler_enable: true
profiler_port: ":6061"
```

## Available Endpoints

When the profiler is enabled, the following endpoints are available:

- `/debug/pprof/` - Index of all available profiles
- `/debug/pprof/profile` - CPU profile (30 seconds by default)
- `/debug/pprof/heap` - Heap profile
- `/debug/pprof/goroutine` - Goroutine profile
- `/debug/pprof/block` - Blocking profile
- `/debug/pprof/mutex` - Mutex contention profile
- `/debug/pprof/trace` - Execution trace

## Command Line Tools

### CPU Profiling
```bash
# Collect 30-second CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile

# Collect 10-second CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10
```

### Heap Profiling
```bash
go tool pprof http://localhost:6060/debug/pprof/heap
```

### Web Interface
```bash
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile
```

## API Reference

### Server

The main type for managing the pprof server.

#### Methods

- `NewServer() *Server` - Creates a new profiler server instance
- `StartIfEnabled(ctx context.Context, cfg config.ConfigurationProfiler) error` - Starts the profiler if enabled in configuration
- `Stop(ctx context.Context) error` - Gracefully stops the profiler server
- `IsRunning() bool` - Returns true if the profiler server is currently running

## Security Considerations

- The profiler should only be enabled in development/staging environments
- Consider firewall rules to restrict access to profiler endpoints
- The profiler exposes sensitive information about your application's internals
- Always disable the profiler in production environments

## Thread Safety

All methods on the Server type are thread-safe and can be called concurrently from multiple goroutines.
