package grpc

import (
	"context"
	"strings"

	"github.com/pitabwire/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/pitabwire/frame/security"
)

const (
	bearerTokenParts     = 2
	grpcAuthHeader       = "authorization"
	grpcAuthSchemeBearer = "bearer"
)

func grpcJwtTokenExtractor(ctx context.Context) (string, error) {
	requestMetadata, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no metadata was saved in context before")
	}

	vv, ok := requestMetadata[grpcAuthHeader]
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no authorization key found in request metadata")
	}

	extractedJwtToken := strings.Split(vv[0], " ")

	if len(extractedJwtToken) != bearerTokenParts ||
		strings.ToLower(extractedJwtToken[0]) != grpcAuthSchemeBearer ||
		extractedJwtToken[1] == "" {
		return "", status.Error(codes.Unauthenticated, "authorization header is invalid")
	}

	return strings.TrimSpace(extractedJwtToken[1]), nil
}

func getGrpcMetadata(ctx context.Context, key string) string {
	requestMetadata, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	vv, ok := requestMetadata[key]
	if !ok {
		return ""
	}

	return vv[0]
}

// UnaryAuthInterceptor Simple grpc interceptor to extract the jwt supplied via authorization bearer token and verify the authentication claims in the token.
func UnaryAuthInterceptor(
	authenticator security.Authenticator,
	opts ...security.AuthOption,
) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		_ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		securityOpts := security.AuthOptions{
			DisableSecurity: false,
		}

		for _, opt := range opts {
			opt(ctx, &securityOpts)
		}

		config := securityOpts.DisableSecurityCfg
		if config != nil {
			securityOpts.DisableSecurity = securityOpts.DisableSecurity && !config.IsRunSecurely()
		}

		if securityOpts.DisableSecurity {
			return handler(ctx, req)
		}

		jwtToken, err := grpcJwtTokenExtractor(ctx)
		if err != nil {
			return nil, err
		}

		ctx, err = authenticator.Authenticate(ctx, jwtToken, opts...)
		if err != nil {
			logger := util.Log(ctx).WithError(err).WithField("jwtToken", jwtToken)
			logger.Info(" UnaryAuthInterceptor -- could not authenticate token")
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		ctx = security.SetupSecondaryClaims(ctx, getGrpcMetadata(ctx, "tenant_id"),
			getGrpcMetadata(ctx, "partition_id"), getGrpcMetadata(ctx, "access_id"),
			getGrpcMetadata(ctx, "contact_id"), getGrpcMetadata(ctx, "session_id"),
			getGrpcMetadata(ctx, "device_id"),
			getGrpcMetadata(ctx, "roles"))

		return handler(ctx, req)
	}
}

// serverStreamWrapper simple wrapper method that stores auth claims for the server stream context.
type serverStreamWrapper struct {
	ctx context.Context
	grpc.ServerStream
}

// Context converts the stream wrappers claims to be contained in the stream context.
func (s *serverStreamWrapper) Context() context.Context {
	return s.ctx
}

// ensureAuthenticatedStreamContext checks if the stream context already has authentication claims.
// If not, and if the service is configured to run securely, it attempts to extract and
// authenticate a JWT from the stream's context. It returns a (potentially wrapped)
// grpc.ServerStream with an updated context and any error encountered.
func ensureAuthenticatedStreamContext(
	ss grpc.ServerStream,
	authenticator security.Authenticator,
	opts ...security.AuthOption,
) (grpc.ServerStream, error) {
	// If claims are already in the context, use the original stream.
	ctx := ss.Context() // Original context from the incoming stream.

	if security.ClaimsFromContext(ctx) != nil {
		return ss, nil
	}

	securityOpts := security.AuthOptions{
		DisableSecurity: false,
	}

	for _, opt := range opts {
		opt(ctx, &securityOpts)
	}

	config := securityOpts.DisableSecurityCfg
	if config != nil {
		securityOpts.DisableSecurity = securityOpts.DisableSecurity && !config.IsRunSecurely()
	}

	if securityOpts.DisableSecurity {
		return ss, nil
	}

	jwtToken, err := grpcJwtTokenExtractor(ctx)
	if err != nil {
		// If token extraction fails, it's an error for secure mode.
		return ss, err // Return original stream and the error.
	}

	// Attempt to authenticate and get an updated context.
	authenticatedCtx, err := authenticator.Authenticate(ctx, jwtToken, opts...)
	if err != nil {
		logger := util.Log(ctx).WithError(err).WithField("jwtToken", jwtToken)
		logger.Info("ensureAuthenticatedStreamContext -- could not authenticate token")
		// Return original stream and the authentication error.
		return ss, status.Error(codes.Unauthenticated, err.Error())
	}

	// Pad partition info if authentication was successful and service runs securely.
	newCtx := security.SetupSecondaryClaims(authenticatedCtx, // Use the authenticated context
		getGrpcMetadata(ss.Context(), "tenant_id"), // Extract metadata from original stream context
		getGrpcMetadata(ss.Context(), "partition_id"),
		getGrpcMetadata(ss.Context(), "access_id"),
		getGrpcMetadata(ss.Context(), "contact_id"),
		getGrpcMetadata(ss.Context(), "session_id"),
		getGrpcMetadata(ss.Context(), "device_id"),
		getGrpcMetadata(ss.Context(), "roles"))

	// Wrap the original stream with newCtx (which is original ctx if not secure or auth failed/skipped, or authenticated ctx if successful).
	// This ensures the handlers always receives a stream from which it can get the correct context.
	return &serverStreamWrapper{newCtx, ss}, nil
}

// StreamAuthInterceptor An authentication claims extractor that will always verify the information flowing in the streams as true jwt claims.
func StreamAuthInterceptor(
	authenticator security.Authenticator,
	opts ...security.AuthOption,
) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		authenticatedStream, err := ensureAuthenticatedStreamContext(ss, authenticator, opts...)
		if err != nil {
			return err
		}
		return handler(srv, authenticatedStream)
	}
}
