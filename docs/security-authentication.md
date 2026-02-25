# Authentication (OAuth2 / JWT)

Frame ships with JWT-based authentication backed by OpenID Connect (OIDC) metadata and JWKS key discovery.

## Overview

- `security.Manager` exposes an `Authenticator`.
- `openid.TokenAuthenticator` fetches and refreshes JWKS keys.
- Claims are extracted into context for downstream use.

## Configuration

Required:

- `OAUTH2_SERVICE_URI`
- `OAUTH2_WELL_KNOWN_OIDC_PATH` (default `.well-known/openid-configuration`)

Optional:

- `OAUTH2_JWT_VERIFY_AUDIENCE`
- `OAUTH2_JWT_VERIFY_ISSUER`

## Authenticate a Token

```go
sm := svc.SecurityManager()
ctx, err := sm.GetAuthenticator(ctx).Authenticate(ctx, token)
```

## Claims in Context

Frame stores claims in context for access by downstream handlers and data models. See `security/security_claims.go` for helpers like:

- `security.ClaimsFromContext(ctx)`

## Server-Side OAuth2 Client Registration

Use `WithRegisterServerOauth2Client()` to register an internal client for service-to-service use.

## Best Practices

- Ensure JWKS URL is reachable.
- Rotate keys safely; the authenticator refreshes JWKS every 5 minutes.
- Validate audience and issuer in production.
