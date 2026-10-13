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

Package: `security/interceptors/connect`, built on `connectrpc.com/connect/v2`.
Every constructor returns a `connect.ServerInterceptor`, which wraps unary and
streaming RPCs alike. Pass them to `connect.NewServer`, register the generated
handlers on that server, and mount it with `connecthttp.Mount`:

```go
import (
    "connectrpc.com/connect/v2"
    "connectrpc.com/connect/v2/connecthttp"

    connectix "github.com/pitabwire/frame/v3/security/interceptors/connect"
)

// otel → validation → auth → tenancy claims, plus your own.
interceptors, err := svc.ConnectDefaultInterceptors(ctx, svc.SecurityManager().GetAuthenticator(ctx),
    connectix.NewTenancyAccessInterceptor(tenancyChecker),
    connectix.NewFunctionAccessInterceptor(functionChecker, procedurePermissions),
)
if err != nil {
    return err
}

server := connect.NewServer(interceptors...)
profilev1connect.RegisterProfileServiceHandler(server, handler)

mux := http.NewServeMux()
connecthttp.Mount(mux, server)
```

| Constructor | Purpose |
| --- | --- |
| `NewAuthInterceptor(authenticator)` | Bearer-token authentication from the `Authorization` header |
| `NewValidationInterceptor(opts...)` | protovalidate rules plus `google.protobuf.Struct` size limits on every received request |
| `NewValidationClientInterceptor(opts...)` | The same validation on the client side, before a request is sent |
| `NewTenancyAccessInterceptor(checker)` | Partition data-access check |
| `NewFunctionAccessInterceptor(checker, perms)` | Per-procedure functional permissions |
| `DefaultList` / `DefaultListWithBinder` | The standard chain; prefer `Service.ConnectDefaultInterceptors` |

`tenancy.NewClaimsInterceptor` and `localization/interceptors/connect.NewLanguageInterceptor`
are server interceptors of the same shape.

Upgrading from frame v2? See the [v2 → v3 migration guide](migrating-v3.md).

## Best Practices

- Always authenticate before authorization.
- Keep token parsing centralized in interceptors.
- Use context-based claims for downstream logic.
