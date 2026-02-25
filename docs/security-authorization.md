# Authorization

Frame provides an authorization abstraction with an adapter for Keto (OpenFGA-like) APIs.

## Concepts

- `Authorizer`: interface for policy evaluation and relation management.
- `CheckRequest`: authorization check input.
- `RelationTuple`: subject-object relationship representation.

## Using the Authorizer

```go
authz := svc.SecurityManager().GetAuthorizer(ctx)
res, err := authz.Check(ctx, security.CheckRequest{
    Subject: security.SubjectRef{Namespace: "user", ObjectID: "u1"},
    Object:  security.ObjectRef{Namespace: "document", ObjectID: "d1"},
    Relation: "viewer",
})
```

## Configuration

Set the authorization service endpoints:

- `AUTHORIZATION_SERVICE_READ_URI`
- `AUTHORIZATION_SERVICE_WRITE_URI`

## Audit Logging

Frame provides an `AuditLogger` interface to record authorization decisions. The Keto adapter uses a default audit logger that can be customized.

## Best Practices

- Separate read vs write endpoints for high availability.
- Use batch checks for performance.
- Record audit logs for sensitive actions.
