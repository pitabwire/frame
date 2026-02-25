package data //nolint:testpackage // consistent with other test files in package

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type DataErrorsSuite struct {
	suite.Suite
}

func TestDataErrorsSuite(t *testing.T) {
	suite.Run(t, new(DataErrorsSuite))
}

func (s *DataErrorsSuite) TestErrorIsDuplicateKey() {
	s.True(ErrorIsDuplicateKey(gorm.ErrDuplicatedKey))
	s.True(ErrorIsDuplicateKey(&pgconn.PgError{Code: "23505"}))
	s.False(ErrorIsDuplicateKey(errors.New("other")))
}

func (s *DataErrorsSuite) TestErrorConvertToAPITable() {
	testCases := []struct {
		name string
		err  error
		want connect.Code
	}{
		{name: "nil", err: nil, want: connect.CodeUnknown},
		{name: "gorm not found", err: gorm.ErrRecordNotFound, want: connect.CodeNotFound},
		{name: "wrapped gorm", err: fmt.Errorf("wrapped: %w", gorm.ErrRecordNotFound), want: connect.CodeNotFound},
		{
			name: "duplicate",
			err:  errors.New("duplicate key value violates unique constraint"),
			want: connect.CodeAlreadyExists,
		},
		{name: "foreign key", err: errors.New("violates foreign key"), want: connect.CodeInvalidArgument},
		{name: "check constraint", err: errors.New("violates check constraint"), want: connect.CodeInvalidArgument},
		{name: "not null", err: errors.New("violates not-null constraint"), want: connect.CodeInvalidArgument},
		{name: "network", err: errors.New("dial tcp timeout"), want: connect.CodeUnavailable},
		{name: "transaction aborted", err: errors.New("transaction deadlock rollback"), want: connect.CodeAborted},
		{name: "fallback", err: errors.New("unknown"), want: connect.CodeInternal},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			got := ErrorConvertToAPI(tc.err)
			if tc.err == nil {
				s.Nil(got)
				return
			}
			s.Require().NotNil(got)
			s.Equal(tc.want, got.Code())
		})
	}
}
