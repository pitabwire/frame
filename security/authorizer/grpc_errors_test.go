package authorizer_test

import (
	"errors"
	"testing"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToGrpcError_Nil(t *testing.T) {
	assert.NoError(t, authorizer.ToGrpcError(nil))
}

func TestToGrpcError_InvalidSubject(t *testing.T) {
	err := authorizer.ToGrpcError(authorizer.ErrInvalidSubject)
	st, ok := status.FromError(err)
	if assert.True(t, ok) {
		assert.Equal(t, codes.Unauthenticated, st.Code())
	}
}

func TestToGrpcError_InvalidObject(t *testing.T) {
	err := authorizer.ToGrpcError(authorizer.ErrInvalidObject)
	st, ok := status.FromError(err)
	if assert.True(t, ok) {
		assert.Equal(t, codes.Unauthenticated, st.Code())
	}
}

func TestToGrpcError_PermissionDenied(t *testing.T) {
	permErr := authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: "t", ID: "1"},
		"view",
		security.SubjectRef{Namespace: "p", ID: "u1"},
		"denied",
	)
	err := authorizer.ToGrpcError(permErr)
	st, ok := status.FromError(err)
	if assert.True(t, ok) {
		assert.Equal(t, codes.PermissionDenied, st.Code())
	}
}

func TestToGrpcError_UnknownError(t *testing.T) {
	err := authorizer.ToGrpcError(errors.New("something broke"))
	st, ok := status.FromError(err)
	if assert.True(t, ok) {
		assert.Equal(t, codes.Internal, st.Code())
	}
}
