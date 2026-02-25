# TLS Configuration

Frame enables TLS when the configuration implements `ConfigurationTLS` and provides certificate paths.

## Required Configuration

Environment variables:

- `TLS_CERTIFICATE_PATH`
- `TLS_CERTIFICATE_KEY_PATH`

If these are present, the HTTP server starts in TLS mode and uses standard HTTP/2.

## Behavior

- Non-TLS mode uses h2c (HTTP/2 without TLS).
- TLS mode uses `http2.ConfigureServer` and a TLS listener.

## Errors

If only one of the TLS paths is provided, Frame returns `ErrTLSPathsNotProvided`.

## Example

```go
cfg := &config.ConfigurationDefault{
    TLSCertificatePath: "/etc/tls/tls.crt",
    TLSCertificateKeyPath: "/etc/tls/tls.key",
}

_, svc := frame.NewService(frame.WithConfig(cfg))
```
