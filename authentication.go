package frame

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dgrijalva/jwt-go"
	"google.golang.org/grpc"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
)

const ctxKeyAuthentication = "authenticationKey"

// AuthenticationClaims Create a struct that will be encoded to a JWT.
// We add jwt.StandardClaims as an embedded type, to provide fields like expiry time
type AuthenticationClaims struct {
	ProfileID   string `json:"profile_id,omitempty"`
	TenantID    string `json:"tenant_id,omitempty"`
	PartitionID string `json:"partition_id,omitempty"`
	AccessID    string `json:"access_id,omitempty"`
	jwt.StandardClaims
}

// AsMetadata Creates a string map to be used as metadata in queue data
func (a *AuthenticationClaims) AsMetadata() map[string]string {

	m := make(map[string]string)
	m["tenant_id"] = a.TenantID
	m["partition_id"] = a.PartitionID
	m["profile_id"] = a.ProfileID
	m["access_id"] = a.AccessID
	return m
}

// ClaimsToContext adds authentication claims to the current supplied context
func (a *AuthenticationClaims) ClaimsToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyAuthentication, a)
}

// ClaimsFromContext extracts authentication claims from the supplied context if any exist
func ClaimsFromContext(ctx context.Context) *AuthenticationClaims {
	authenticationClaims, ok := ctx.Value(ctxKeyAuthentication).(*AuthenticationClaims)
	if !ok {
		return nil
	}

	return authenticationClaims
}

// ClaimsFromMap extracts authentication claims from the supplied map if they exist
func ClaimsFromMap(m map[string]string) *AuthenticationClaims {
	var authenticationClaims AuthenticationClaims

	if val, ok := m["tenant_id"]; ok {
		authenticationClaims = AuthenticationClaims{}
		authenticationClaims.TenantID = val

		if val, ok := m["partition_id"]; ok {
			authenticationClaims.PartitionID = val

			if val, ok := m["profile_id"]; ok {
				authenticationClaims.ProfileID = val

				if val, ok := m["access_id"]; ok {
					authenticationClaims.AccessID = val
					return &authenticationClaims
				}
			}
		}
	}

	return nil
}

func authenticate(ctx context.Context, jwtToken string) (context.Context, error) {

	claims := &AuthenticationClaims{}

	token, err := jwt.ParseWithClaims(jwtToken, claims, getPemCert)
	if err != nil {
		return ctx, err
	}

	if !token.Valid {
		return ctx, errors.New("supplied token was invalid")
	}

	ctx = claims.ClaimsToContext(ctx)

	return ctx, nil

}

type jwks struct {
	Keys []jsonWebKeys `json:"keys"`
}

type jsonWebKeys struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5c []string `json:"x5c"`
}

func getPemCert(token *jwt.Token) (interface{}, error) {

	wellKnownJWKUrl := GetEnv("AUTHENTICATION_WELL_KNOWN_JWK_URL", "https://oauth2.api.antinvestor.com/.well-known/jwks.json")

	resp, err := http.Get(wellKnownJWKUrl)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jwks = jwks{}
	err = json.NewDecoder(resp.Body).Decode(&jwks)

	if err != nil {
		return nil, err
	}

	cert := ""
	for k, _ := range jwks.Keys {
		if token.Header["kid"] == jwks.Keys[k].Kid {
			cert = "-----BEGIN CERTIFICATE-----\n" + jwks.Keys[k].X5c[0] + "\n-----END CERTIFICATE-----"
		}
	}

	if cert == "" {
		return cert, errors.New("Unable to find appropriate key.")
	}

	return cert, nil
}

// AuthenticationMiddleware Simple http middleware function
// to verify and extract authentication data supplied in a jwt as authorization bearer token
func AuthenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")

		if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, "Bearer ") {
			http.Error(w, "An authorization header is required", http.StatusForbidden)
			return
		}

		extractedJwtToken := strings.Split(authorizationHeader, "Bearer ")

		if len(extractedJwtToken) != 2 {
			http.Error(w, "Malformed Authorization header", http.StatusBadRequest)
			return
		}

		jwtToken := strings.TrimSpace(extractedJwtToken[1])

		ctx := r.Context()
		ctx, err := authenticate(ctx, jwtToken)

		if err != nil {
			http.Error(w, "Authorization header is invalid", http.StatusUnauthorized)
			return
		}

		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

func grpcJwtTokenExtractor(ctx context.Context) (string, error) {
	requestMetadata, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata was saved in context before")
	}

	vv, ok := requestMetadata["authorization"]
	if !ok {
		return "", errors.New("no authorization key found in request metadata")
	}

	authorizationHeader := vv[0]

	if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, "bearer ") {
		return "", errors.New("an authorization header is required")
	}

	extractedJwtToken := strings.Split(authorizationHeader, "bearer ")

	if len(extractedJwtToken) != 2 {
		return "", errors.New("malformed Authorization header")
	}

	return strings.TrimSpace(extractedJwtToken[1]), nil
}

// UnaryAuthInterceptor Simple grpc interceptor to extract the jwt supplied via authorization bearer token and verify the authentication claims in the token
func UnaryAuthInterceptor(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

	jwtToken, err := grpcJwtTokenExtractor(ctx)
	if err != nil {
		return nil, err
	}

	ctx, err = authenticate(ctx, jwtToken)
	if err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// serverStreamWrapper simple wrapper method that stores auth claims for the server stream context
type serverStreamWrapper struct {
	authClaim *AuthenticationClaims
	grpc.ServerStream
}

func (s *serverStreamWrapper) Context() context.Context {
	return s.authClaim.ClaimsToContext(s.Context())
}

// StreamAuthInterceptor An authentication claims extractor that will always verify the information flowing in the streams as true jwt claims
func StreamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {

	localServerStream := ss
	authClaim := ClaimsFromContext(localServerStream.Context())
	if authClaim == nil {
		ctx := ss.Context()
		jwtToken, err := grpcJwtTokenExtractor(ctx)
		if err != nil {
			return err
		}

		ctx, err = authenticate(ctx, jwtToken)
		if err != nil {
			return err
		}

		authClaim = ClaimsFromContext(ctx)

		localServerStream = &serverStreamWrapper{authClaim, ss}
	}
	return handler(srv, localServerStream)
}
