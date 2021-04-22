package frame

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAuthenticationFromContext(t *testing.T) {
	ctx := context.Background()
	claims := ClaimsFromContext(ctx)

	assert.Nil(t, claims, "A context without claims should not produce one")

	claims = &AuthenticationClaims{}
	ctx = context.WithValue(ctx, ctxKeyAuthentication, claims)
	assert.NotNil(t, claims, "A context with claims should produce one")
}

