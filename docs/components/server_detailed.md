# Server Components

## Overview

Frame provides robust server implementations for both HTTP and gRPC protocols. The server components are designed to be highly configurable while maintaining simplicity in basic usage scenarios.

## HTTP Server

### Features

1. **Routing**
   - Built on gorilla/mux
   - Path parameters
   - Query string handling
   - Route middleware
   
2. **Middleware Support**
   - Authentication
   - Request logging
   - CORS
   - Rate limiting
   - Panic recovery

3. **Health Checks**
   - Built-in health check endpoints
   - Customizable health checks
   - Readiness probes
   - Liveness probes

### Basic Setup

```go
func main() {
    router := mux.NewRouter()
    
    // Add routes
    router.HandleFunc("/api/v1/users", GetUsers).Methods("GET")
    router.HandleFunc("/api/v1/users/{id}", GetUser).Methods("GET")
    
    // Create HTTP server
    server := frame.HttpHandler(router)
    
    // Create service with server
    service := frame.NewService("api-service", server)
    
    // Run service
    if err := service.Run(ctx, ":8080"); err != nil {
        log.Fatal(err)
    }
}
```

### Advanced Configuration

```go
func main() {
    router := mux.NewRouter()
    
    // Custom request logger
    reqLogger := requestlog.NewNCSALogger(os.Stdout, func(e error) {
        log.Printf("Request log error: %v", e)
    })
    
    // Server options
    opts := &server.Options{
        RequestLogger:         reqLogger,
        ReadHeaderTimeout:     time.Second * 5,
        ReadTimeout:          time.Second * 30,
        WriteTimeout:         time.Second * 30,
        IdleTimeout:          time.Second * 120,
        MaxHeaderBytes:       1 << 20,
    }
    
    // Create HTTP server with options
    server := frame.HttpHandler(router, frame.HttpOptions(opts))
    
    // ... rest of setup
}
```

## gRPC Server

### Features

1. **Protocol Buffer Support**
   - Automatic serialization/deserialization
   - Type safety
   - Backward compatibility

2. **Streaming**
   - Unary RPC
   - Server streaming
   - Client streaming
   - Bi-directional streaming

3. **Interceptors**
   - Authentication
   - Logging
   - Error handling
   - Metrics collection

### Basic Setup

```go
func main() {
    // Create gRPC server
    grpcServer := grpc.NewServer()
    
    // Register services
    pb.RegisterUserServiceServer(grpcServer, &userService{})
    
    // Create frame server
    server := frame.GrpcServer(grpcServer)
    
    // Create service
    service := frame.NewService("grpc-service", server)
    
    // Run service
    if err := service.Run(ctx, ":50051"); err != nil {
        log.Fatal(err)
    }
}
```

### Advanced Configuration

```go
func main() {
    // Create interceptors
    unaryInterceptor := grpc.UnaryInterceptor(
        grpc_middleware.ChainUnaryServer(
            grpc_auth.UnaryServerInterceptor(authenticate),
            grpc_recovery.UnaryServerInterceptor(),
            grpc_validator.UnaryServerInterceptor(),
        ),
    )
    
    streamInterceptor := grpc.StreamInterceptor(
        grpc_middleware.ChainStreamServer(
            grpc_auth.StreamServerInterceptor(authenticate),
            grpc_recovery.StreamServerInterceptor(),
            grpc_validator.StreamServerInterceptor(),
        ),
    )
    
    // Create server with options
    grpcServer := grpc.NewServer(
        unaryInterceptor,
        streamInterceptor,
        grpc.MaxRecvMsgSize(1024*1024*10),
        grpc.MaxSendMsgSize(1024*1024*10),
    )
    
    // ... rest of setup
}
```

## Best Practices

### 1. Error Handling

```go
func handler(w http.ResponseWriter, r *http.Request) {
    data, err := processRequest(r)
    if err != nil {
        switch e := err.(type) {
        case *ValidationError:
            http.Error(w, e.Error(), http.StatusBadRequest)
        case *AuthError:
            http.Error(w, e.Error(), http.StatusUnauthorized)
        default:
            http.Error(w, "Internal server error", http.StatusInternalServerError)
        }
        return
    }
    
    json.NewEncoder(w).Encode(data)
}
```

### 2. Middleware Usage

```go
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf(
            "%s %s %s",
            r.Method,
            r.RequestURI,
            time.Since(start),
        )
    })
}

router.Use(loggingMiddleware)
```

### 3. Context Usage

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Add timeout
    ctx, cancel := context.WithTimeout(ctx, time.Second*5)
    defer cancel()
    
    select {
    case result := <-processAsync(ctx):
        json.NewEncoder(w).Encode(result)
    case <-ctx.Done():
        http.Error(w, "Request timeout", http.StatusGatewayTimeout)
    }
}
```

### 4. Health Checks

```go
type HealthCheck struct {
    Status    string `json:"status"`
    Timestamp string `json:"timestamp"`
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
    check := HealthCheck{
        Status:    "healthy",
        Timestamp: time.Now().UTC().Format(time.RFC3339),
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(check)
}

router.HandleFunc("/health", healthCheckHandler)
```

## Security Considerations

1. **TLS Configuration**
   ```go
   tlsConfig := &tls.Config{
       MinVersion:               tls.VersionTLS12,
       CurvePreferences:        []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
       PreferServerCipherSuites: true,
   }
   ```

2. **CORS Configuration**
   ```go
   c := cors.New(cors.Options{
       AllowedOrigins:   []string{"https://api.example.com"},
       AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
       AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
       AllowCredentials: true,
       MaxAge:          300,
   })
   
   router.Use(c.Handler)
   ```

3. **Rate Limiting**
   ```go
   limiter := rate.NewLimiter(rate.Every(time.Second), 100)
   
   router.Use(func(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           if !limiter.Allow() {
               http.Error(w, "Too many requests", http.StatusTooManyRequests)
               return
           }
           next.ServeHTTP(w, r)
       })
   })
   ```
