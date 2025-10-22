package connect

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/util"
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

// AuthInterceptor implements connect.Interceptor for JWT authentication.
type AuthInterceptor struct {
	authenticator security.Authenticator
}

// NewAuthInterceptor creates a new authentication interceptor.
func NewAuthInterceptor(authenticator security.Authenticator) *AuthInterceptor {

	return &AuthInterceptor{
		authenticator: authenticator,
	}
}

// extractToken extracts and validates the bearer token from the header.
func (a *AuthInterceptor) extractToken(authHeader string) (string, error) {
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
func (a *AuthInterceptor) authenticate(ctx context.Context, authHeader string) (context.Context, error) {

	logger := util.Log(ctx).WithField("has_auth_header", authHeader != "")

	token, err := a.extractToken(authHeader)
	if err != nil {
		logger.WithError(err).Debug("failed to extract authentication token")
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Authenticate the token
	authCtx, err := a.authenticator.Authenticate(ctx, token)
	if err != nil {
		logger.WithError(err).Info("authentication failed")
		return nil, connect.NewError(connect.CodeUnauthenticated, ErrInvalidToken)
	}

	return authCtx, nil
}

// WrapUnary implements the unary interceptor for authentication.
func (a *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		authCtx, err := a.authenticate(ctx, req.Header().Get(authorizationKey))
		if err != nil {
			return nil, err
		}

		return next(authCtx, req)
	}
}

// WrapStreamingClient implements the streaming client interceptor (pass-through for server-side).
func (a *AuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements the streaming handler interceptor for authentication.
func (a *AuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		authCtx, err := a.authenticate(ctx, conn.RequestHeader().Get(authorizationKey))
		if err != nil {
			return err
		}

		return next(authCtx, conn)
	}
}
