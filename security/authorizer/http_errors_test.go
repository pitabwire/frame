package authorizer_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/stretchr/testify/assert"
)

func TestToHTTPStatusCode_Nil(t *testing.T) {
	assert.Equal(t, http.StatusOK, authorizer.ToHTTPStatusCode(nil))
}

func TestToHTTPStatusCode_InvalidSubject(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, authorizer.ToHTTPStatusCode(authorizer.ErrInvalidSubject))
}

func TestToHTTPStatusCode_InvalidObject(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, authorizer.ToHTTPStatusCode(authorizer.ErrInvalidObject))
}

func TestToHTTPStatusCode_PermissionDenied(t *testing.T) {
	err := authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: "t", ID: "1"},
		"view",
		security.SubjectRef{Namespace: "p", ID: "u1"},
		"denied",
	)
	assert.Equal(t, http.StatusForbidden, authorizer.ToHTTPStatusCode(err))
}

func TestToHTTPStatusCode_UnknownError(t *testing.T) {
	assert.Equal(t, http.StatusInternalServerError, authorizer.ToHTTPStatusCode(errors.New("something broke")))
}
