# Data Utilities

The `data` package provides common data helpers used across Frame.

## BaseModel

`data.BaseModel` implements GORM lifecycle hooks and includes tenancy fields.

```go
type User struct {
    data.BaseModel
    Name string
}
```

Features:
- ID generation
- Created/Modified timestamps
- Versioning
- Tenancy fields (`TenantID`, `PartitionID`, `AccessID`)

## DSN

`data.DSN` provides DSN validation helpers used across datastore, queue, and cache.

## Errors

- `ErrorIsNoRows(err)` checks for common "not found" database errors.
- `frame.ErrorIsNotFound(err)` adds gRPC and connect support.

## Search Helpers

The `data/search.go` package provides helpers for query filtering and pagination.

## Best Practices

- Use `BaseModel` for consistent tenancy metadata.
- Validate DSNs before using them in configuration.
