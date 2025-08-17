package frameauth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	bearerScheme         = "Bearer"
	bearerTokenParts     = 2
	grpcAuthHeader       = "authorization"
	grpcAuthSchemeBearer = "bearer"
)

// Jwks represents JSON Web Key Set structure
type Jwks struct {
	Keys []JSONWebKeys `json:"keys"`
}

// JSONWebKeys represents a JSON Web Key
type JSONWebKeys struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5c []string `json:"x5c"`
}

// authenticator implements the Authenticator interface
type authenticator struct {
	config Config
	logger Logger
}

// NewAuthenticator creates a new authenticator instance
func NewAuthenticator(config Config, logger Logger) Authenticator {
	return &authenticator{
		config: config,
		logger: logger,
	}
}

// IsEnabled returns whether authentication is enabled
func (a *authenticator) IsEnabled() bool {
	if a.config == nil {
		return false
	}
	return a.config.IsRunSecurely()
}

// Authenticate validates a JWT token and returns an updated context with claims
func (a *authenticator) Authenticate(ctx context.Context, jwtToken string, audience string, issuer string) (context.Context, error) {
	claims := &AuthenticationClaims{}

	var options []jwt.ParserOption

	if audience != "" {
		options = append(options, jwt.WithAudience(audience))
	}

	if issuer != "" {
		options = append(options, jwt.WithIssuer(issuer))
	}

	token, err := jwt.ParseWithClaims(jwtToken, claims, a.getPemCert, options...)
	if err != nil {
		return ctx, err
	}

	if !token.Valid {
		return ctx, errors.New("supplied token was invalid")
	}

	ctx = jwtToContext(ctx, jwtToken)
	ctx = claims.ClaimsToContext(ctx)

	return ctx, nil
}

// getPemCert retrieves the PEM certificate for JWT verification
func (a *authenticator) getPemCert(token *jwt.Token) (any, error) {
	if a.config.GetOauth2WellKnownJwkData() == "" {
		return nil, errors.New("json web key data is not available")
	}

	wellKnownJWK := a.config.GetOauth2WellKnownJwkData()

	var jwks = Jwks{}
	err := json.NewDecoder(strings.NewReader(wellKnownJWK)).Decode(&jwks)
	if err != nil {
		return nil, err
	}

	for k, val := range jwks.Keys {
		if token.Header["kid"] == jwks.Keys[k].Kid {
			var exponent []byte
			if exponent, err = base64.RawURLEncoding.DecodeString(val.E); err != nil {
				return nil, err
			}

			// Decode the modulus from Base64.
			var modulus []byte
			if modulus, err = base64.RawURLEncoding.DecodeString(val.N); err != nil {
				return nil, err
			}

			// Create the RSA public key.
			publicKey := &rsa.PublicKey{}

			// Turn the exponent into an integer.
			//
			// According to RFC 7517, these numbers are in big-endian format.
			// https://tools.ietf.org/html/rfc7517#appendix-A.1
			expUint64 := big.NewInt(0).SetBytes(exponent).Uint64()
			// Check for potential overflow before converting to int. int(^uint(0) >> 1) is math.MaxInt.
			if expUint64 > uint64(int(^uint(0)>>1)) {
				return nil, fmt.Errorf("exponent value %d from token is too large to fit in int type", expUint64)
			}
			publicKey.E = int(expUint64)

			// Turn the modulus into a *big.Int.
			publicKey.N = big.NewInt(0).SetBytes(modulus)

			return publicKey, nil
		}
	}

	return nil, errors.New("unable to find appropriate key")
}

// HTTPMiddleware returns an HTTP middleware for authentication
func (a *authenticator) HTTPMiddleware(audience string, issuer string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !a.IsEnabled() {
				next.ServeHTTP(w, r)
				return
			}

			authorizationHeader := r.Header.Get("Authorization")

			logger := a.logger.WithField("authorization_header", authorizationHeader)

			if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, "Bearer ") {
				logger.WithField("available_headers", r.Header).
					Debug("AuthenticationMiddleware -- could not authenticate missing token")
				http.Error(w, "An authorization header is required", http.StatusForbidden)
				return
			}

			extractedJwtToken := strings.Split(authorizationHeader, " ")

			if len(extractedJwtToken) != bearerTokenParts {
				logger.Debug("AuthenticationMiddleware -- token format is not valid")
				http.Error(w, "Malformed Authorization header", http.StatusBadRequest)
				return
			}

			jwtToken := strings.TrimSpace(extractedJwtToken[1])

			ctx := r.Context()
			ctx, err := a.Authenticate(ctx, jwtToken, audience, issuer)

			if err != nil {
				logger.WithError(err).Info("AuthenticationMiddleware -- could not authenticate token")
				http.Error(w, "Authorization header is invalid", http.StatusUnauthorized)
				return
			}

			ctx = a.systemPadPartitionInfo(ctx,
				r.Header.Get("Tenant_id"), r.Header.Get("Partition_id"),
				r.Header.Get("Access_id"), r.Header.Get("Contact_id"),
				r.Header.Get("Session_id"), r.Header.Get("Device_id"), r.Header.Get("Roles"))

			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// systemPadPartitionInfo pads partition information for internal systems
func (a *authenticator) systemPadPartitionInfo(
	ctx context.Context,
	tenantID, partitionID, accessID, contactID, sessionID, deviceID, roles string,
) context.Context {
	claims := ClaimsFromContext(ctx)

	// If no claims or not an internal system, no padding is needed.
	if claims == nil || !claims.isInternalSystem() {
		return ctx
	}

	val := claims.GetTenantID()
	if val == "" {
		claims.TenantID = tenantID
	}

	val = claims.GetPartitionID()
	if val == "" {
		claims.PartitionID = partitionID
	}

	val = claims.GetAccessID()
	if val == "" {
		claims.AccessID = accessID
	}

	val = claims.GetContactID()
	if val == "" {
		claims.ContactID = contactID
	}

	val = claims.GetSessionID()
	if val == "" {
		claims.SessionID = sessionID
	}

	val = claims.GetDeviceID()
	if val == "" {
		claims.DeviceID = deviceID
	}

	claimRoles := claims.GetRoles()
	if len(claimRoles) == 0 {
		claims.Roles = strings.Split(roles, ",")
	}

	return claims.ClaimsToContext(ctx)
}

// grpcJwtTokenExtractor extracts JWT token from gRPC metadata
func grpcJwtTokenExtractor(ctx context.Context) (string, error) {
	requestMetadata, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no metadata was saved in context before")
	}

	vv, ok := requestMetadata["authorization"]
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no authorization key found in request metadata")
	}

	extractedJwtToken := strings.Split(vv[0], " ")

	if len(extractedJwtToken) != 2 ||
		strings.ToLower(extractedJwtToken[0]) != "bearer" ||
		extractedJwtToken[1] == "" {
		return "", status.Error(codes.Unauthenticated, "authorization header is invalid")
	}

	return strings.TrimSpace(extractedJwtToken[1]), nil
}

// getGrpcMetadata extracts metadata from gRPC context
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

// UnaryInterceptor returns a gRPC unary server interceptor for authentication
func (a *authenticator) UnaryInterceptor(audience string, issuer string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		_ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if a.IsEnabled() {
			jwtToken, err := grpcJwtTokenExtractor(ctx)
			if err != nil {
				return nil, err
			}

			ctx, err = a.Authenticate(ctx, jwtToken, audience, issuer)
			if err != nil {
				logger := a.logger.WithError(err).WithField("jwtToken", jwtToken)
				logger.Info("UnaryAuthInterceptor -- could not authenticate token")
				return nil, status.Error(codes.Unauthenticated, err.Error())
			}

			ctx = a.systemPadPartitionInfo(ctx, getGrpcMetadata(ctx, "tenant_id"),
				getGrpcMetadata(ctx, "partition_id"), getGrpcMetadata(ctx, "access_id"),
				getGrpcMetadata(ctx, "contact_id"), getGrpcMetadata(ctx, "session_id"),
				getGrpcMetadata(ctx, "device_id"),
				getGrpcMetadata(ctx, "roles"))
		}
		return handler(ctx, req)
	}
}

// serverStreamWrapper simple wrapper method that stores auth claims for the server stream context
type serverStreamWrapper struct {
	ctx context.Context
	grpc.ServerStream
}

// Context converts the stream wrappers claims to be contained in the stream context
func (s *serverStreamWrapper) Context() context.Context {
	return s.ctx
}

// ensureAuthenticatedStreamContext checks if the stream context already has authentication claims
func (a *authenticator) ensureAuthenticatedStreamContext(
	ss grpc.ServerStream,
	audience string,
	issuer string,
) (grpc.ServerStream, error) {
	// If claims are already in the context, use the original stream.
	if ClaimsFromContext(ss.Context()) != nil {
		return ss, nil
	}

	ctx := ss.Context() // Original context from the incoming stream.
	newCtx := ctx       // Initialize newCtx with the original context; it will be updated if authentication succeeds.

	if a.IsEnabled() {
		jwtToken, err := grpcJwtTokenExtractor(ctx)
		if err != nil {
			// If token extraction fails, it's an error for secure mode.
			return ss, err // Return original stream and the error.
		}

		// Attempt to authenticate and get an updated context.
		authenticatedCtx, err := a.Authenticate(ctx, jwtToken, audience, issuer)
		if err != nil {
			logger := a.logger.WithError(err).WithField("jwtToken", jwtToken)
			logger.Info("ensureAuthenticatedStreamContext -- could not authenticate token")
			// Return original stream and the authentication error.
			return ss, status.Error(codes.Unauthenticated, err.Error())
		}
		newCtx = authenticatedCtx // Update newCtx with the context from successful authentication.

		// Pad partition info if authentication was successful and service runs securely.
		newCtx = a.systemPadPartitionInfo(newCtx, // Use the authenticated context
			getGrpcMetadata(ss.Context(), "tenant_id"), // Extract metadata from original stream context
			getGrpcMetadata(ss.Context(), "partition_id"),
			getGrpcMetadata(ss.Context(), "access_id"),
			getGrpcMetadata(ss.Context(), "contact_id"),
			getGrpcMetadata(ss.Context(), "session_id"),
			getGrpcMetadata(ss.Context(), "device_id"),
			getGrpcMetadata(ss.Context(), "roles"))
	}

	// Wrap the original stream with newCtx
	return &serverStreamWrapper{newCtx, ss}, nil
}

// StreamInterceptor returns a gRPC stream server interceptor for authentication
func (a *authenticator) StreamInterceptor(audience string, issuer string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		authenticatedStream, err := a.ensureAuthenticatedStreamContext(ss, audience, issuer)
		if err != nil {
			return err
		}
		return handler(srv, authenticatedStream)
	}
}
