package authorizer_test

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
)

func TestToConnectError_Nil(t *testing.T) {
	assert.NoError(t, authorizer.ToConnectError(nil))
}

func TestToConnectError_InvalidSubject(t *testing.T) {
	err := authorizer.ToConnectError(authorizer.ErrInvalidSubject)
	var cErr *connect.Error
	if assert.ErrorAs(t, err, &cErr) {
		assert.Equal(t, connect.CodeUnauthenticated, cErr.Code())
	}
}

func TestToConnectError_InvalidObject(t *testing.T) {
	err := authorizer.ToConnectError(authorizer.ErrInvalidObject)
	var cErr *connect.Error
	if assert.ErrorAs(t, err, &cErr) {
		assert.Equal(t, connect.CodeUnauthenticated, cErr.Code())
	}
}

func TestToConnectError_PermissionDenied(t *testing.T) {
	permErr := authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: "t", ID: "1"},
		"view",
		security.SubjectRef{Namespace: "p", ID: "u1"},
		"denied",
	)
	err := authorizer.ToConnectError(permErr)
	var cErr *connect.Error
	if assert.ErrorAs(t, err, &cErr) {
		assert.Equal(t, connect.CodePermissionDenied, cErr.Code())
	}
}

func TestToConnectError_UnknownError(t *testing.T) {
	err := authorizer.ToConnectError(errors.New("something broke"))
	var cErr *connect.Error
	if assert.ErrorAs(t, err, &cErr) {
		assert.Equal(t, connect.CodeInternal, cErr.Code())
	}
}
