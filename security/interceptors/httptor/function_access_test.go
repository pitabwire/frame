package httptor_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pitabwire/frame/security/interceptors/httptor"
)

func TestFunctionAccessMiddleware_BoundaryRespected(t *testing.T) {
	perms := map[string][]string{
		"/api/profile": {"profile_read"},
	}

	// Handler that records if it was called
	called := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	middleware := httptor.FunctionAccessMiddleware(handler, nil, perms)

	// Partial segment: /api/profileadmin should NOT match /api/profile
	// → no permissions found → passes through to handler
	called = false
	req := httptest.NewRequest(http.MethodGet, "/api/profileadmin", nil)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)
	assert.True(t, called, "/api/profileadmin should pass through (no match = allow)")

	// Sub-path: /api/profile/123 should match /api/profile
	// → permissions found → checker called → fails without claims → 401
	called = false
	req = httptest.NewRequest(http.MethodGet, "/api/profile/123", nil)
	w = httptest.NewRecorder()
	middleware.ServeHTTP(w, req)
	assert.False(t, called, "/api/profile/123 should be intercepted by permission check")
	assert.Equal(t, http.StatusUnauthorized, w.Code, "should return 401 without claims")
}

func TestFunctionAccessMiddleware_UnmappedPathAllowed(t *testing.T) {
	perms := map[string][]string{
		"/api/profile": {"profile_read"},
	}

	called := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	middleware := httptor.FunctionAccessMiddleware(handler, nil, perms)

	req := httptest.NewRequest(http.MethodGet, "/other/path", nil)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)
	assert.True(t, called, "unmapped path should be allowed through")
}

func TestFunctionAccessMiddleware_ExactMatch(t *testing.T) {
	perms := map[string][]string{
		"/api/profile": {"profile_read"},
	}

	called := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	middleware := httptor.FunctionAccessMiddleware(handler, nil, perms)

	// Exact match: /api/profile should match
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)
	assert.False(t, called, "exact match should be intercepted")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
