package authorizer

import (
	"errors"
	"fmt"

	"github.com/pitabwire/frame/security"
)

var (
	// ErrPermissionDenied indicates the subject lacks the required permission.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrInvalidObject indicates an invalid object reference.
	ErrInvalidObject = errors.New("invalid object reference")

	// ErrInvalidSubject indicates an invalid subject reference.
	ErrInvalidSubject = errors.New("invalid subject reference")

	// ErrTupleNotFound indicates the relationship tuple was not found.
	ErrTupleNotFound = errors.New("relationship tuple not found")

	// ErrTupleAlreadyExists indicates the relationship tuple already exists.
	ErrTupleAlreadyExists = errors.New("relationship tuple already exists")

	// ErrAuthzServiceDown indicates the authorization service is unavailable.
	ErrAuthzServiceDown = errors.New("authorization service unavailable")

	// ErrInvalidPermission indicates an invalid permission was requested.
	ErrInvalidPermission = errors.New("invalid permission")

	// ErrInvalidRole indicates an invalid role was specified.
	ErrInvalidRole = errors.New("invalid role")
)

// PermissionDeniedError provides detailed denial information.
type PermissionDeniedError struct {
	Object     security.ObjectRef
	Permission string
	Subject    security.SubjectRef
	Reason     string
}

// Error implements the error interface.
func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf("permission denied: %s cannot %s on %s:%s - %s",
		e.Subject.ID, e.Permission, e.Object.Namespace, e.Object.ID, e.Reason)
}

// Is allows checking if an error is a PermissionDeniedError.
func (e *PermissionDeniedError) Is(target error) bool {
	return target == ErrPermissionDenied
}

// Unwrap returns the base error for error wrapping support.
func (e *PermissionDeniedError) Unwrap() error {
	return ErrPermissionDenied
}

// AuthzServiceError wraps authorization service errors with context.
type AuthzServiceError struct {
	Operation string
	Cause     error
}

// Error implements the error interface.
func (e *AuthzServiceError) Error() string {
	return fmt.Sprintf("authz service error during %s: %v", e.Operation, e.Cause)
}

// Is allows checking error type.
func (e *AuthzServiceError) Is(target error) bool {
	return target == ErrAuthzServiceDown
}

// Unwrap returns the cause for error wrapping support.
func (e *AuthzServiceError) Unwrap() error {
	return e.Cause
}

// NewPermissionDeniedError creates a new PermissionDeniedError.
func NewPermissionDeniedError(
	object security.ObjectRef,
	permission string,
	subject security.SubjectRef,
	reason string,
) *PermissionDeniedError {
	return &PermissionDeniedError{
		Object:     object,
		Permission: permission,
		Subject:    subject,
		Reason:     reason,
	}
}

// NewAuthzServiceError creates a new AuthzServiceError.
func NewAuthzServiceError(operation string, cause error) *AuthzServiceError {
	return &AuthzServiceError{
		Operation: operation,
		Cause:     cause,
	}
}
