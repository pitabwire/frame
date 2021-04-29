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
	ProfileID      string `json:"profile_id,omitempty"`
	TenantID       string `json:"tenant_id,omitempty"`
	PartitionID    string `json:"partition_id,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
	jwt.StandardClaims
}

func (a *AuthenticationClaims) AsMetadata() map[string]string {

	m := make(map[string]string)
	m["tenant_id"] = a.TenantID
	m["partition_id"] = a.PartitionID
	m["profile_id"] = a.ProfileID
	return m
}

func (a *AuthenticationClaims) ClaimsToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyAuthentication, a)
}

func ClaimsFromContext(ctx context.Context) *AuthenticationClaims {
	authenticationClaims, ok := ctx.Value(ctxKeyAuthentication).(*AuthenticationClaims)
	if !ok {
		return nil
	}

	return authenticationClaims
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

func UnaryAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

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

func StreamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {

	ctx := ss.Context()
	jwtToken, err := grpcJwtTokenExtractor(ctx)
	if err != nil {
		return err
	}

	ctx, err = authenticate(ctx, jwtToken)
	if err != nil {
		return err
	}
	return handler(srv, ss)
}
