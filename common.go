package frame

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pitabwire/frame/data"
)

// ErrIsNotFound checks if an error represents a "not found" condition.
// It handles multiple error types:
// - Database errors: gorm.ErrRecordNotFound, sql.ErrNoRows (via ErrorIsNoRows)
// - gRPC errors: codes.NotFound
// - Generic errors: error messages containing "not found" (case-insensitive).
func ErrIsNotFound(err error) bool {
	if err == nil {
		return false
	}

	// Check database errors using existing ErrorIsNoRows function
	if data.ErrorIsNoRows(err) {
		return true
	}

	// Check gRPC status errors
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.NotFound
	}

	// Check error message for "not found" string (case-insensitive)
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "notfound") ||
		strings.Contains(errMsg, "404") {
		return true
	}

	return false
}
