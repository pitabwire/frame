# Migrating from frame v2 to v3

Frame v3 moves to [connect-go v2](https://github.com/connectrpc/connect-go/blob/v2.0.0/docs/v2-migration.md)
(`connectrpc.com/connect/v2`). Frame's public API exposes Connect interceptor and
error types, so the module path changes to `github.com/pitabwire/frame/v3`.

Nothing else in frame changes shape: options, datastore, queue, events, cache,
telemetry and the HTTP server behave as in v2.1.8. If a service does not use
frame's Connect interceptors or `data.ErrorConvertToAPI`, the import-path change
is the whole migration.

## 1. Order of work

Run these steps in each service, one service at a time:

1. Rewrite frame imports: `github.com/pitabwire/frame/v2` becomes `github.com/pitabwire/frame/v3`.
2. Install the connect v2 tools:
   ```sh
   go install connectrpc.com/connect/v2/cmd/connect-go-v2-migrate@v1.0.0
   go install connectrpc.com/connect/v2/cmd/protoc-gen-connect-go@v2.0.0
   ```
   The migrate tool is versioned on its own (`v1.0.0`); `@v2.0.0` does not resolve.
3. Run `connect-go-v2-migrate -w`. It updates `buf.gen.yaml` and rewrites code
   that does not depend on generated code.
4. Re-generate code (`buf generate`). With BSR generated SDKs, depend on the
   `…/connectrpc/go/v2` module. In `buf.gen.yaml`, use `buf.build/connectrpc/go:v2.0.0`
   for the remote plugin and drop the `simple` option, since v2 always
   generates the simple style.
5. Run `connect-go-v2-migrate -w` again. This rewrites handler signatures and
   client calls against the new generated code.
6. Fix the frame call sites listed in sections 3 and 4.
7. Update dependencies and tidy:
   ```sh
   go get github.com/pitabwire/frame/v3@v3.0.0 \
       connectrpc.com/connect/v2@v2.0.0 connectrpc.com/otelconnect@v0.12.0
   go mod tidy
   ```
   If you use them, also get `connectrpc.com/validate@v0.9.0`,
   `connectrpc.com/grpchealth/v2@v2.0.0` and `connectrpc.com/grpcreflect/v2@v2.0.0`.
8. Run `go build ./... && go vet ./... && go test ./...`, then work through the
   warnings the tool printed.

## 2. Import paths

| v2 | v3 |
| --- | --- |
| `github.com/pitabwire/frame/v2` | `github.com/pitabwire/frame/v3` |
| `github.com/pitabwire/frame/v2/<pkg>` | `github.com/pitabwire/frame/v3/<pkg>` |
| `connectrpc.com/connect` | `connectrpc.com/connect/v2` (core types) |
| (in `connectrpc.com/connect`) | `connectrpc.com/connect/v2/connecthttp` (mounting, transports, HTTP options) |
| (in `connectrpc.com/connect`) | `connectrpc.com/connect/v2/connectproto` (error details, codecs) |
| `connectrpc.com/grpchealth` | `connectrpc.com/grpchealth/v2` |
| `connectrpc.com/grpcreflect` | `connectrpc.com/grpcreflect/v2` |
| `connectrpc.com/otelconnect` | unchanged path, v0.12.0 or later |
| `connectrpc.com/validate` | unchanged path, v0.9.0 or later |

To rewrite the frame imports in one go:

```sh
grep -rl 'github.com/pitabwire/frame/v2' --include='*.go' . \
  | xargs sed -i 's#github.com/pitabwire/frame/v2#github.com/pitabwire/frame/v3#g'
```

## 3. Frame API changes

Each frame Connect interceptor is now a `connect.ServerInterceptor`, a function
type that wraps unary and streaming RPCs alike. The v1 `connect.Interceptor`
interface, with `WrapUnary`, `WrapStreamingClient` and `WrapStreamingHandler`,
no longer exists.

| Symbol | v2 (connect v1) | v3 (connect v2) |
| --- | --- | --- |
| `Service.ConnectDefaultInterceptors` | `(ctx, auth, more ...connect.Interceptor) ([]connect.Interceptor, error)` | `(ctx, auth, more ...connect.ServerInterceptor) ([]connect.ServerInterceptor, error)` |
| `security/interceptors/connect.DefaultList` | `(ctx, auth, more ...connect.Interceptor) ([]connect.Interceptor, error)` | `(ctx, auth, more ...connect.ServerInterceptor) ([]connect.ServerInterceptor, error)` |
| `…/connect.DefaultListWithBinder` | `(ctx, auth, binder, more ...connect.Interceptor) ([]connect.Interceptor, error)` | `(ctx, auth, binder, more ...connect.ServerInterceptor) ([]connect.ServerInterceptor, error)` |
| `…/connect.NewAuthInterceptor` | `(security.Authenticator) connect.Interceptor` | `(security.Authenticator) connect.ServerInterceptor` |
| `…/connect.NewValidationInterceptor` | `(...Option) connect.Interceptor` (client and server) | `(...Option) connect.ServerInterceptor` |
| `…/connect.NewValidationClientInterceptor` | n/a | `(...Option) connect.ClientInterceptor` (new: client-side validation) |
| `…/connect.NewTenancyAccessInterceptor` | `(*authorizer.TenancyAccessChecker) connect.Interceptor` | `(*authorizer.TenancyAccessChecker) connect.ServerInterceptor` |
| `…/connect.NewFunctionAccessInterceptor` | `(*authorizer.FunctionChecker, map[string][]string) connect.Interceptor` | `(*authorizer.FunctionChecker, map[string][]string) connect.ServerInterceptor` |
| `tenancy.NewClaimsInterceptor` | `() connect.Interceptor` | `() connect.ServerInterceptor` |
| `tenancy.NewClaimsInterceptorWithBinder` | `(*ClaimsBinder) connect.Interceptor` | `(*ClaimsBinder) connect.ServerInterceptor` |
| `localization/interceptors/connect.NewLanguageInterceptor` | `() (*LanguageInterceptor, error)` | `() (connect.ServerInterceptor, error)` |
| `localization/interceptors/connect.LanguageInterceptor` | exported struct | removed (use the constructor) |
| `data.ErrorConvertToAPI` | `(error) *connect.Error` (connect v1) | `(error) *connect.Error` (connect v2) |
| `security/authorizer.ToConnectError` | returns a connect v1 `*connect.Error` | returns a connect v2 `*connect.Error` |

Behavior is unchanged in these respects:

- Error codes and wire messages are the same. `data.ErrorConvertToAPI` and
  `authorizer.ToConnectError` still send `err.Error()` as the message. The
  original error is now attached with `WithCause`, so `errors.Is` and
  `errors.As` match it on the server.
- The default chain order is unchanged: otel, then validation, then auth, then
  tenancy claims, then your own.

These changes affect streaming RPCs:

- The auth, tenancy-access, function-access, claims and language interceptors
  now run once per RPC for every RPC type. They pass their enriched context
  (claims, tenancy, languages) to streaming handlers as well. In v2 the
  language interceptor did not propagate to streaming handlers.
- Validation wraps the server stream. It checks every received request message,
  and every sent response message when `WithValidateResponses` is set.

### Before and after: wiring a service

```go
// frame v2 + connect v1
import (
    "connectrpc.com/connect"
    "github.com/pitabwire/frame/v2"
)

interceptors, err := svc.ConnectDefaultInterceptors(ctx, authenticator)
if err != nil {
    return err
}
path, handler := profilev1connect.NewProfileServiceHandler(
    impl,
    connect.WithInterceptors(interceptors...),
)
mux := http.NewServeMux()
mux.Handle(path, handler)
svc.Init(ctx, frame.WithHTTPHandler(mux))
```

```go
// frame v3 + connect v2
import (
    "connectrpc.com/connect/v2"
    "connectrpc.com/connect/v2/connecthttp"
    "github.com/pitabwire/frame/v3"
)

interceptors, err := svc.ConnectDefaultInterceptors(ctx, authenticator)
if err != nil {
    return err
}
server := connect.NewServer(interceptors...)
profilev1connect.RegisterProfileServiceHandler(server, impl)

mux := http.NewServeMux()
connecthttp.Mount(mux, server)
svc.Init(ctx, frame.WithHTTPHandler(mux))
```

Interceptors now belong to the `*connect.Server`, not to each handler. If two
services need different interceptor chains, give each its own server and mount
both on the same mux.

### Before and after: adding frame's access interceptors

```go
// v2
interceptors, err := svc.ConnectDefaultInterceptors(ctx, authenticator,
    connectix.NewTenancyAccessInterceptor(tenancyChecker),
    connectix.NewFunctionAccessInterceptor(functionChecker, perms),
)
opts := connect.WithInterceptors(interceptors...)
```

```go
// v3: the same call; the result goes to connect.NewServer.
interceptors, err := svc.ConnectDefaultInterceptors(ctx, authenticator,
    connectix.NewTenancyAccessInterceptor(tenancyChecker),
    connectix.NewFunctionAccessInterceptor(functionChecker, perms),
)
server := connect.NewServer(interceptors...)
```

### Before and after: language interceptor

```go
// v2
li, err := languageix.NewLanguageInterceptor()
opts := connect.WithInterceptors(li)
```

```go
// v3
li, err := languageix.NewLanguageInterceptor()
server := connect.NewServer(li)
```

### Before and after: a custom interceptor

```go
// v1 interface
type auditInterceptor struct{}

func (auditInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
    return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
        log(ctx, req.Spec().Procedure, req.Header().Get("X-Request-Id"))
        return next(ctx, req)
    }
}
func (auditInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc { return next }
func (auditInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
    return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
        log(ctx, conn.Spec().Procedure, conn.RequestHeader().Get("X-Request-Id"))
        return next(ctx, conn)
    }
}
```

```go
// v2 function type: one body covers unary and streaming.
func newAuditInterceptor() connect.ServerInterceptor {
    return func(next connect.ServerFunc) connect.ServerFunc {
        return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
            if info, ok := connect.CallInfoForServerContext(ctx); ok {
                log(ctx, spec.Procedure, info.RequestHeader().Get("X-Request-Id"))
            }
            return next(ctx, spec, stream)
        }
    }
}
```

To act on each message, wrap the `connect.ServerStream` before you call `next`,
and override `Receive` and/or `Send`. Frame's validation interceptor works this
way. On the client side, a `connect.ClientInterceptor` wraps the
`connect.ClientStream` that `next(ctx, spec)` returns.

## 4. connect v2 call-site changes

The migrate tool handles most of these. They are listed here so that you can
review its output.

### Handlers: no request or response wrappers

```go
// v1
func (s *server) Get(ctx context.Context, req *connect.Request[v1.GetRequest]) (*connect.Response[v1.GetResponse], error) {
    tenant := req.Header().Get("X-Tenant-Id")
    return connect.NewResponse(&v1.GetResponse{Id: req.Msg.GetId()}), nil
}
```

```go
// v2
func (s *server) Get(ctx context.Context, req *v1.GetRequest) (*v1.GetResponse, error) {
    info, _ := connect.CallInfoForServerContext(ctx)
    tenant := info.RequestHeader().Get("X-Tenant-Id")
    return &v1.GetResponse{Id: req.GetId()}, nil
}
```

Response headers and trailers: `info.ResponseHeader().Set(...)` and
`info.ResponseTrailer().Set(...)`. `connect.Header` is not a `net/http.Header`.
It has `Get`, `Set`, `Add`, `Values`, `Delete` and `All`.

### Clients

```go
// v1
client := profilev1connect.NewProfileServiceClient(httpClient, baseURL, connect.WithGRPC())
req := connect.NewRequest(&v1.GetRequest{Id: id})
req.Header().Set("Authorization", "Bearer "+token)
res, err := client.Get(ctx, req)
name := res.Msg.GetName()
```

```go
// v2
client := profilev1connect.NewProfileServiceClient(connect.NewClient(
    connecthttp.NewTransport(httpClient, baseURL, connecthttp.WithGRPC()),
    // client interceptors go here, e.g. otelconnect.NewClientInterceptor()
))
ctx, info := connect.NewClientContext(ctx)
info.RequestHeader().Set("Authorization", "Bearer "+token)
res, err := client.Get(ctx, &v1.GetRequest{Id: id})
name := res.GetName()
```

### Errors

Only a `*connect.Error` that you build yourself reaches the wire. A plain
`error` returned from a handler now arrives at the client as `CodeUnknown` with
no message. Return frame's converters (`data.ErrorConvertToAPI`,
`authorizer.ToConnectError`) or a `connect.NewError`.

```go
// v1
return nil, connect.NewError(connect.CodeNotFound, err)
return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bad id %q", id))
```

```go
// v2: the message is a string. Attach the error as a local cause.
return nil, connect.NewError(connect.CodeNotFound, err.Error()).WithCause(err)
return nil, connect.NewError(connect.CodeInvalidArgument, "name is required")
return nil, connect.Errorf(connect.CodeInvalidArgument, "bad id %q", id)

// To hide internals from callers:
return nil, connect.NewError(connect.CodeInternal, "internal error").WithCause(err)
```

Error details: `connect.NewErrorDetail` becomes `connectproto.NewErrorDetail`,
and `cerr.AddDetail(d)` becomes `cerr = cerr.WithDetail(d)`.
`connect.CodeOf` and the `connect.Code*` constants are unchanged.

### Streaming

```go
// v1 handler
func (s *server) Watch(ctx context.Context, req *connect.Request[v1.WatchRequest], stream *connect.ServerStream[v1.Event]) error

// v2 handler: the generated per-RPC stream type
func (s *server) Watch(ctx context.Context, req *v1.WatchRequest, stream v1connect.WatchServiceWatchServerStream) error
```

The v1 receive loop `for stream.Receive() { stream.Msg() }; stream.Err()`
becomes `msg, err := stream.Receive()` until `errors.Is(err, io.EOF)`. On the
client, the stream constructors return `(stream, error)`. `CloseRequest`
becomes `CloseSend`, and `CloseResponse` becomes `Close`.

### Options that moved to connecthttp

`WithReadMaxBytes`, `WithSendMaxBytes`, `WithCompressMinBytes`, `WithGRPC`,
`WithGRPCWeb`, `WithProtoJSON`, `WithHTTPGet`, `WithSendGzip` and
`WithSendCompression` keep their names and move to `connecthttp`. Handler options go on
`connecthttp.Mount(mux, server, opts...)`. Client options go on
`connecthttp.NewTransport(httpClient, baseURL, opts...)`.

### Read limit (behavior change)

connect v2 caps each received message at **4 MiB** by default. In v1 there was
no cap. Larger messages fail with `CodeResourceExhausted`. The migrate tool
keeps the v1 behavior by pinning `connecthttp.WithReadMaxBytes(0)` at each call
site. Instead, set an explicit limit wherever you need large messages (for
example, file uploads), and drop the pin everywhere else:

```go
connecthttp.Mount(mux, server, connecthttp.WithReadMaxBytes(32<<20))
```

### Ecosystem packages

- `otelconnect.NewInterceptor()` splits into `otelconnect.NewServerInterceptor()`
  and `otelconnect.NewClientInterceptor()`, and both return an error. Frame's
  default chain already uses the server form.
- `validate.NewInterceptor()` splits into `validate.NewServerInterceptor()` and
  `validate.NewClientInterceptor()`. Frame's own `NewValidationInterceptor`
  covers the same protovalidate rules and adds Struct limits, so you don't need both.
- grpchealth: `mux.Handle(grpchealth.NewHandler(checker))` becomes `grpchealth.Register(server, checker)`.
- grpcreflect: `NewHandlerV1` and `NewHandlerV1Alpha` become `grpcreflect.Register(server)`.
- `connectrpc.com/authn` has no v2 release. Use frame's `NewAuthInterceptor` or
  `httptor.AuthenticationMiddleware` instead.

## 5. Testing

v2 clients don't depend on HTTP. In tests, `connectinprocess` connects a client
straight to a server with your real interceptor chain, with no listener and no
`httptest.Server`:

```go
interceptors, err := connectix.DefaultList(ctx, authenticator)
server := connect.NewServer(interceptors...)
profilev1connect.RegisterProfileServiceHandler(server, impl)
client := profilev1connect.NewProfileServiceClient(
    connect.NewClient(connectinprocess.New(server)),
)
```

## 6. Other changes in v3.0.0

- Every dependency is upgraded to its latest stable release. This includes
  OpenTelemetry 1.47 (`go.opentelemetry.io/otel/log` is now stable at v1.47.0),
  grpc 1.84, pgx 5.11 and protovalidate 1.4. The toolchain is go1.26.9.
- `security/openid`: JWKS keys with `kty` EC and OKP now decode. The `JWK`
  struct used to declare `crv` and `x` twice, so `encoding/json` dropped both.
  The `OKPCrv` and `OKPX` fields are removed, and OKP keys use `Crv` and `X`.
- The HTTP server enables HTTP/2 through `http.Server.Protocols` alone. It no
  longer calls the deprecated `x/net/http2.ConfigureServer`.
