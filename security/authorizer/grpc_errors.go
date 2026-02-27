package authorizer

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ToGrpcError translates authorization errors into gRPC status errors.
//
// Mapping:
//   - ErrInvalidSubject / ErrInvalidObject → codes.Unauthenticated
//   - PermissionDeniedError → codes.PermissionDenied
//   - everything else → codes.Internal
func ToGrpcError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrInvalidSubject) || errors.Is(err, ErrInvalidObject) {
		return status.Error(codes.Unauthenticated, err.Error())
	}

	var permErr *PermissionDeniedError
	if errors.As(err, &permErr) {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return status.Error(codes.Internal, err.Error())
}
