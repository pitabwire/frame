package frame

import (
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pitabwire/frame/data"
)

// ErrorIsNotFound checks if an error represents a "not found" condition.
// It handles multiple error types:
// - Database errors: gorm.ErrRecordNotFound, sql.ErrNoRows (via ErrorIsNoRows)
// - gRPC errors: codes.NotFound
// - Generic errors: error messages containing "not found" (case-insensitive).
func ErrorIsNotFound(err error) bool {
	if err == nil {
		return false
	}

	// Check database errors using existing ErrorIsNoRows function
	if data.ErrorIsNoRows(err) {
		return true
	}

	// Check gRPC status errors
	if gErr, ok := status.FromError(err); ok {
		return gErr.Code() == codes.NotFound
	}

	// Check connect status errors
	if connect.CodeOf(err) == connect.CodeNotFound {
		return true
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
