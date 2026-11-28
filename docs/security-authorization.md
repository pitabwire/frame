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

## Tenancy access self-heal

`authorizer.NewTenancyAccessChecker` checks Plane-1 data access on
`{namespace}:{tenant}/{partition}`. When an internal service bot (role
`internal`) is denied, the checker can self-heal by writing a `#service`
tuple and retrying once (the default callback, or `WithOnTenancyAccessDenied`).

Internal callers may act on any tenancy named in `X-Tenant-Id` /
`X-Partition-Id` headers, and queue consumers rebuild claims from message
metadata. Neither is proof that a partition exists, so self-heal only runs
when:

1. the caller's **verified JWT** (`security.VerifiedTokenClaimsFromContext`,
   set only by the JWT authenticator) was issued for that tenant and partition
   (`partition_id` or `partition_ids`), or
2. a `PartitionVerifierFunc` confirms the partition exists.

The default verifier accepts a partition when Keto already holds a `member`
tuple for it: a direct membership or a parent link
(`child#member <- parent#member`). `service` tuples do not count, because
self-heal writes them. Verifier errors fail closed.

Behaviour to plan for with the default verifier:

- A **new child partition** is unknown until the tenancy service's
  asynchronous partition sync (`authz.partition.sync`) writes its parent
  link. Requests that arrive in that window are denied.
- A **root partition nobody has been granted on** has no `member` tuple
  and is denied until access is granted or the service-bot bootstrap
  provisions explicit `#service` grants.

Mitigations:

- Prefer an authoritative verifier backed by the tenancy service, e.g.
  `WithPartitionVerifier(func(ctx, t, p) (bool, error) { /* GetPartition */ })`.
- Add a bounded retry for the sync window:
  `WithPartitionVerifyRetry(3, 200*time.Millisecond)` (0.2s + 0.4s at most,
  interrupted when the request context ends).

## Best Practices

- Separate read vs write endpoints for high availability.
- Use batch checks for performance.
- Record audit logs for sensitive actions.
