package connect

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v3/security"
)

const (
	bearerScheme       = "Bearer "
	authorizationKey   = "Authorization"
	tokenPartsExpected = 2
)

var (
	// ErrMissingToken is returned when no authorization header is present.
	ErrMissingToken = errors.New("authorization header is required")
	// ErrMalformedToken is returned when the authorization header is malformed.
	ErrMalformedToken = errors.New("malformed authorization header")
	// ErrInvalidToken is returned when token authentication fails.
	ErrInvalidToken = errors.New("invalid authorization token")
)

// authInterceptor authenticates the bearer token on every inbound RPC.
type authInterceptor struct {
	authenticator security.Authenticator
}

// NewAuthInterceptor returns a server interceptor that authenticates the
// bearer token in the Authorization header of every RPC, unary and streaming.
// The authenticated context, enriched with the X-Tenant-Id, X-Partition-Id
// and X-Access-Id tenancy hints, is passed to the handler.
func NewAuthInterceptor(authenticator security.Authenticator) connect.ServerInterceptor {
	a := &authInterceptor{
		authenticator: authenticator,
	}
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			info, ok := connect.CallInfoForServerContext(ctx)
			if !ok {
				return connect.NewError(connect.CodeUnauthenticated, ErrMissingToken.Error()).WithCause(ErrMissingToken)
			}

			headers := info.RequestHeader()
			authCtx, err := a.authenticate(ctx, headers.Get(authorizationKey))
			if err != nil {
				return err
			}

			authCtx = security.EnrichTenancyClaims(authCtx,
				headers.Get("X-Tenant-Id"),
				headers.Get("X-Partition-Id"),
				headers.Get("X-Access-Id"))

			return next(authCtx, spec, stream)
		}
	}
}

// extractToken extracts and validates the bearer token from the header.
func (a *authInterceptor) extractToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingToken
	}

	if !strings.HasPrefix(authHeader, bearerScheme) {
		return "", ErrMalformedToken
	}

	token := strings.TrimPrefix(authHeader, bearerScheme)
	token = strings.TrimSpace(token)

	if token == "" {
		return "", ErrMalformedToken
	}

	return token, nil
}

// authenticate performs the authentication check.
func (a *authInterceptor) authenticate(ctx context.Context, authHeader string) (context.Context, error) {
	token, err := a.extractToken(authHeader)
	if err != nil {
		util.Log(ctx).WithError(err).Debug("failed to extract authentication token")
		return nil, connect.NewError(connect.CodeUnauthenticated, err.Error()).WithCause(err)
	}

	// Authenticate the token
	authCtx, err := a.authenticator.Authenticate(ctx, token)
	if err != nil {
		util.Log(ctx).WithField("has_auth_header", authHeader != "").WithError(err).Info("authentication failed")
		return nil, connect.NewError(connect.CodeUnauthenticated, ErrInvalidToken.Error()).WithCause(ErrInvalidToken)
	}

	return authCtx, nil
}
