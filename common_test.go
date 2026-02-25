package frame //nolint:testpackage // tests access package internals

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type ErrorSuite struct {
	suite.Suite
}

func TestErrorSuite(t *testing.T) {
	suite.Run(t, new(ErrorSuite))
}

func (s *ErrorSuite) TestErrorIsNotFoundTable() {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "sql no rows", err: sql.ErrNoRows, want: true},
		{name: "gorm record not found", err: gorm.ErrRecordNotFound, want: true},
		{name: "grpc not found", err: status.Error(codes.NotFound, "missing"), want: true},
		{name: "grpc internal", err: status.Error(codes.Internal, "failed"), want: false},
		{name: "connect not found", err: connect.NewError(connect.CodeNotFound, errors.New("missing")), want: true},
		{name: "text not found", err: errors.New("resource not found"), want: true},
		{name: "text compact notfound", err: errors.New("resource_notfound"), want: true},
		{name: "text 404", err: fmt.Errorf("status=%d", 404), want: true},
		{name: "other error", err: errors.New("boom"), want: false},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Equal(tc.want, ErrorIsNotFound(tc.err))
		})
	}
}
