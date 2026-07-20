package queue

import "errors"

// Sentinel errors used by push HTTP status mapping and handlers.
var (
	// ErrNotRetryable indicates permanent business failure. Maps to HTTP 422.
	// Cloud Tasks still retries non-2xx until maxAttempts — ops must configure DLQ.
	ErrNotRetryable = errors.New("queue: not retryable")

	// ErrDecode indicates malformed inbound payload / protocol decode failure. Maps to 400.
	ErrDecode = errors.New("queue: decode failed")

	// ErrTooLarge indicates the request body exceeded the configured limit. Maps to 413.
	ErrTooLarge = errors.New("queue: body too large")

	// ErrUnauthorized indicates missing or invalid credentials. Maps to 401.
	ErrUnauthorized = errors.New("queue: unauthorized")

	// ErrForbidden indicates valid credentials that fail audience/issuer checks. Maps to 403.
	ErrForbidden = errors.New("queue: forbidden")
)
