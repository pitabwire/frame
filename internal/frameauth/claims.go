package frameauth

import (
	"github.com/pitabwire/frame/internal/common"
)

// contextKey type moved to common interfaces to avoid duplication

const (
	ctxKeyAuthenticationClaim       = common.ContextKey("authenticationClaimKey")
	ctxKeySkipTenancyCheckOnClaim   = common.ContextKey("skipTenancyCheckOnClaimKey")
	ctxKeyAuthenticationJwt         = common.ContextKey("authenticationJwtKey")
)

// AuthenticationClaims type moved to authentication.go to avoid duplication

// All AuthenticationClaims methods moved to authentication.go to avoid duplication

// SkipTenancyChecksOnClaims and IsTenancyChecksOnClaimSkipped moved to authentication.go to avoid duplication

// ClaimsFromContext moved to common interfaces to avoid duplication

// All duplicate functions moved to authentication.go to avoid duplication
