# Security Interceptors and Middleware

Frame provides authentication and validation interceptors for HTTP, gRPC, and Connect.

## HTTP Middleware

Package: `security/interceptors/httptor`

```go
mw := httptor.AuthenticationMiddleware(svc.SecurityManager())
_, svc := frame.NewService(
    frame.WithHTTPHandler(router),
    frame.WithHTTPMiddleware(mw),
)
```

`ContextSetupMiddleware` also enriches context with request metadata.

## gRPC Interceptors

Package: `security/interceptors/grpc`

```go
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(grpcAuth.UnaryAuthInterceptor(svc.SecurityManager(), svc.Config())),
)
```

## Connect Interceptors

Package: `security/interceptors/connect`

```go
interceptor := connectAuth.NewAuthInterceptor(svc.SecurityManager(), svc.Config())
```

## Validation Interceptors

Connect interceptors include protobuf validation helpers.

## Best Practices

- Always authenticate before authorization.
- Keep token parsing centralized in interceptors.
- Use context-based claims for downstream logic.
