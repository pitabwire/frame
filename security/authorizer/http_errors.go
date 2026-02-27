package authorizer

import (
	"errors"
	"net/http"
)

// ToHTTPStatusCode translates authorization errors into HTTP status codes.
//
// Mapping:
//   - ErrInvalidSubject / ErrInvalidObject → 401 Unauthorized
//   - PermissionDeniedError → 403 Forbidden
//   - everything else → 500 Internal Server Error
func ToHTTPStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	if errors.Is(err, ErrInvalidSubject) || errors.Is(err, ErrInvalidObject) {
		return http.StatusUnauthorized
	}

	var permErr *PermissionDeniedError
	if errors.As(err, &permErr) {
		return http.StatusForbidden
	}

	return http.StatusInternalServerError
}
