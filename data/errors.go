package data

import (
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"gorm.io/gorm"
)

// ErrorIsNoRows validate if supplied error is because of record missing in DB.
func ErrorIsNoRows(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows)
}

// gormErrorMap maps known GORM errors to Connect RPC codes.
//nolint:gochecknoglobals // this mapping allows for efficient error conversion
var gormErrorMap = map[error]connect.Code{
	gorm.ErrRecordNotFound:      connect.CodeNotFound,
	gorm.ErrInvalidTransaction:  connect.CodeFailedPrecondition,
	gorm.ErrNotImplemented:      connect.CodeUnimplemented,
	gorm.ErrMissingWhereClause:  connect.CodeInvalidArgument,
	gorm.ErrUnsupportedRelation: connect.CodeInvalidArgument,
	gorm.ErrPrimaryKeyRequired:  connect.CodeInvalidArgument,
	gorm.ErrModelValueRequired:  connect.CodeInvalidArgument,
}

// ErrorConvertToAPI converts GORM errors to appropriate Connect RPC errors efficiently.
func ErrorConvertToAPI(err error) error {
	if err == nil {
		return nil
	}

	// Fast path: exact match for known GORM errors
	if code, found := gormErrorMap[err]; found {
		return connect.NewError(code, err)
	}

	// For wrapped errors (common with gorm), unwrap and check
	for unwrapped := err; unwrapped != nil; unwrapped = errors.Unwrap(unwrapped) {
		if code, found := gormErrorMap[unwrapped]; found {
			return connect.NewError(code, err)
		}
	}

	// Normalize error message once
	msg := strings.ToLower(err.Error())

	// Constraint violations
	switch {
	case containsAny(msg, "duplicate key", "unique constraint", "violates unique constraint"):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case containsAny(msg, "foreign key", "violates foreign key"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case containsAny(msg, "check constraint", "violates check constraint"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case containsAny(msg, "not null", "violates not-null constraint"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Connection / network issues
	if containsAny(msg, "connection refused", "connection reset", "timeout", "network", "dial tcp") {
		return connect.NewError(connect.CodeUnavailable, err)
	}

	// Transaction issues (deadlock, rollback, etc.)
	if strings.Contains(msg, "transaction") &&
		containsAny(msg, "rollback", "aborted", "deadlock") {
		return connect.NewError(connect.CodeAborted, err)
	}

	// Fallback
	return connect.NewError(connect.CodeInternal, err)
}

// containsAny checks if any of the substrings exist in s (avoids repeated calls).
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
