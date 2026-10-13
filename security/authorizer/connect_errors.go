package authorizer

import (
	"errors"

	"connectrpc.com/connect/v2"
)

// ToConnectError translates authorization errors into ConnectRPC error codes.
//
// Mapping:
//   - ErrInvalidSubject / ErrInvalidObject → CodeUnauthenticated
//   - PermissionDeniedError → CodePermissionDenied
//   - everything else → CodeInternal
func ToConnectError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrInvalidSubject) || errors.Is(err, ErrInvalidObject) {
		return connect.NewError(connect.CodeUnauthenticated, err.Error()).WithCause(err)
	}

	var permErr *PermissionDeniedError
	if errors.As(err, &permErr) {
		return connect.NewError(connect.CodePermissionDenied, err.Error()).WithCause(err)
	}

	return connect.NewError(connect.CodeInternal, err.Error()).WithCause(err)
}
