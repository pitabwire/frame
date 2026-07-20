package push

import (
	"context"
	"errors"
	"net/http"

	"github.com/pitabwire/frame/v2/queue"
)

// HTTPStatusFor maps processing errors to HTTP status codes for push delivery.
func HTTPStatusFor(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, queue.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, queue.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, queue.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, queue.ErrDecode):
		return http.StatusBadRequest
	case errors.Is(err, queue.ErrNotRetryable):
		return http.StatusUnprocessableEntity
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	default:
		return http.StatusServiceUnavailable
	}
}
