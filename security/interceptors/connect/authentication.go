package connect

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/security"
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

// authInterceptor implements connect.validationInterceptor for JWT authentication.
type authInterceptor struct {
	authenticator security.Authenticator
}

// NewAuthInterceptor creates a new authentication interceptor.
func NewAuthInterceptor(authenticator security.Authenticator) connect.Interceptor {
	return &authInterceptor{
		authenticator: authenticator,
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Authenticate the token
	authCtx, err := a.authenticator.Authenticate(ctx, token)
	if err != nil {
		util.Log(ctx).WithField("has_auth_header", authHeader != "").WithError(err).Info("authentication failed")
		return nil, connect.NewError(connect.CodeUnauthenticated, ErrInvalidToken)
	}

	return authCtx, nil
}

// WrapUnary implements the unary interceptor for authentication.
func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		headers := req.Header()
		authCtx, err := a.authenticate(ctx, headers.Get(authorizationKey))
		if err != nil {
			return nil, err
		}

		secCtx := security.SetupSecondaryClaims(authCtx,
			headers.Get("X-Tenant-Id"), headers.Get("X-Partition-Id"),
			headers.Get("X-Profile-Id"), headers.Get("X-Access-Id"),
			headers.Get("X-Contact-Id"), headers.Get("X-Session-Id"),
			headers.Get("X-Device-Id"), headers.Get("X-Roles"))

		return next(secCtx, req)
	}
}

// WrapStreamingClient implements the streaming client interceptor (pass-through for server-side).
func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements the streaming handler interceptor for authentication.
func (a *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		headers := conn.RequestHeader()
		authCtx, err := a.authenticate(ctx, headers.Get(authorizationKey))
		if err != nil {
			return err
		}

		secCtx := security.SetupSecondaryClaims(authCtx,
			headers.Get("X-Tenant-Id"), headers.Get("X-Partition-Id"),
			headers.Get("X-Profile-Id"), headers.Get("X-Access-Id"),
			headers.Get("X-Contact-Id"), headers.Get("X-Session-Id"),
			headers.Get("X-Device-Id"), headers.Get("X-Roles"))

		return next(secCtx, conn)
	}
}
