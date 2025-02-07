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

Let's create a simple user management API that demonstrates Frame's key features:
- HTTP Server setup
- Database integration
- Authentication
- Request handling

```go
package main

import (
    "context"
    "encoding/json"
    "github.com/gorilla/mux"
    "github.com/pitabwire/frame"
    "log"
    "net/http"
    "time"
)

// User represents our user model
type User struct {
    ID        uint      `json:"id" gorm:"primarykey"`
    Name      string    `json:"name"`
    Email     string    `json:"email" gorm:"uniqueIndex"`
    CreatedAt time.Time `json:"created_at"`
}

// UserHandler handles user-related HTTP requests
type UserHandler struct {
    service *frame.Service
}

// CreateUser handles user creation
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var user User
    if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Get database from frame
    db := frame.GetDB(r.Context())
    if err := db.Create(&user).Error; err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(user)
}

// GetUser handles fetching a user
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]

    var user User
    db := frame.GetDB(r.Context())
    if err := db.First(&user, id).Error; err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(user)
}

func main() {
    ctx := context.Background()

    // Setup database
    dbURL := "postgres://user:password@localhost:5432/myapp?sslmode=disable"
    dbOption := frame.Datastore(ctx, dbURL, false)

    // Create router and handler
    router := mux.NewRouter()
    handler := &UserHandler{}

    // Setup routes
    api := router.PathPrefix("/api/v1").Subrouter()
    api.HandleFunc("/users", handler.CreateUser).Methods("POST")
    api.HandleFunc("/users/{id}", handler.GetUser).Methods("GET")

    // Add authentication middleware
    authConfig := frame.AuthConfig{
        JWTSecret: []byte("your-secret-key"),
        TokenExpiration: 24 * time.Hour,
    }
    authMiddleware := frame.NewAuthMiddleware(authConfig)
    api.Use(authMiddleware.Handler)

    // Create HTTP server
    server := frame.HttpHandler(router)

    // Create and run service
    service := frame.NewService("user-service",
        server,     // HTTP server
        dbOption,   // Database
    )

    // Auto-migrate database
    db := frame.GetDB(ctx)
    if err := db.AutoMigrate(&User{}); err != nil {
        log.Fatal("Failed to migrate database:", err)
    }

    // Start the service
    if err := service.Run(ctx, ":8080"); err != nil {
        log.Fatal("Failed to start service:", err)
    }
}
```

This example demonstrates:

1. **Service Setup**
   - Creating an HTTP server with routing
   - Configuring database connection
   - Setting up authentication

2. **Database Integration**
   - Model definition with GORM tags
   - Auto-migration
   - Basic CRUD operations

3. **Authentication**
   - JWT-based authentication
   - Protected routes
   - Middleware integration

4. **Request Handling**
   - Route definition
   - Request parsing
   - Response formatting

To use this example:

1. Install Frame:
   ```bash
   go get -u github.com/pitabwire/frame
   ```

2. Set up PostgreSQL database

3. Run the application:
   ```bash
   go run main.go
   ```

4. Test the API:
   ```bash
   # Create a user
   curl -X POST -H "Authorization: Bearer <token>" \
        -H "Content-Type: application/json" \
        -d '{"name":"John Doe","email":"john@example.com"}' \
        http://localhost:8080/api/v1/users

   # Get a user
   curl -H "Authorization: Bearer <token>" \
        http://localhost:8080/api/v1/users/1
   ```

For more detailed documentation on each component, check the sections below.

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
