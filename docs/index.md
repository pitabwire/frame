## Frame

A simple frame for quickly setting up api servers based on [go-cloud](https://github.com/google/go-cloud) framework

### Overview

Frame lets you do anything you want to do your way. It organizes and simplifies access to the things 
you utilize within your setup. Only what is initialized at startup is what frame will instantiate at runtime.
Under the hood, frame utilizes [go-cloud](https://github.com/google/go-cloud) to be cloud native without a lot of worries on being locked in.

### Documentation

#### Core Documentation
- [Architecture Overview](architecture.md) - Core design principles, components, and best practices
- [Quick Start Guide](components/index.md) - Get started with Frame quickly

#### Component Documentation
- [Server Components](components/server_detailed.md)
  - HTTP Server Implementation
  - gRPC Server Implementation
  - Middleware Support
  - Configuration Examples

- [Datastore](components/datastore_detailed.md)
  - Database Integration with GORM
  - Multi-tenancy Support
  - Migration Management
  - Performance Optimization

- [Queue System](components/queue_detailed.md)
  - Message Queue Implementation
  - Multiple Backend Support
  - Publisher/Subscriber Patterns
  - Monitoring and Metrics

#### Security
- [Authentication & Authorization](security/authentication_authorization.md)
  - OAuth2 and JWT Support
  - Role-Based Access Control
  - Security Best Practices
  - Token Management

### Quick start
```
go get -u github.com/pitabwire/frame
```

### Quick Start Example

The simplest possible Frame server in 5 lines:

```go
package main
import ("github.com/pitabwire/frame"; "net/http"; "context")
func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("Hello!")) })
    frame.NewService("hello-service", frame.HttpHandler(http.DefaultServeMux)).Run(context.Background(), ":8080")
}
```

Try it:
```bash
curl http://localhost:8080/
```

For more comprehensive examples and detailed documentation of Frame's features, check the sections below.

### Contribution

Join us in delivering a better frame! by:

Spreading the word
   - Simply tell people who might be interested about it
   - Help others solve blockers on Stack Overflow and Github Issues
   - Write missing tutorials and suggesting improvements
   - Promote frame on GitHub by Starring and Watching the repository

Program
   - Create a pull request on Github to fix issues, new features
   - Create issues on Github whenever something is broken or needs to be improved on
