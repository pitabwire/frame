package frame

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"math/big"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
)

const ctxKeyAuthenticationClaim = contextKey("authenticationClaimKey")
const ctxKeyAuthenticationJwt = contextKey("authenticationJwtKey")

// JwtToContext adds authentication jwt to the current supplied context
func jwtToContext(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, ctxKeyAuthenticationJwt, jwt)
}

// JwtFromContext extracts authentication jwt from the supplied context if any exist
func JwtFromContext(ctx context.Context) string {
	jwtString, ok := ctx.Value(ctxKeyAuthenticationJwt).(string)
	if !ok {
		return ""
	}

	return jwtString
}

// AuthenticationClaims Create a struct that will be encoded to a JWT.
// We add jwt.StandardClaims as an embedded type, to provide fields like expiry time
type AuthenticationClaims struct {
	Ext         map[string]any `json:"ext,omitempty"`
	TenantID    string         `json:"tenant_id,omitempty"`
	PartitionID string         `json:"partition_id,omitempty"`
	AccessID    string         `json:"access_id,omitempty"`
	ContactID   string         `json:"contact_id,omitempty"`
	Roles       []string       `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

func (a *AuthenticationClaims) GetTenantId() string {

	result := a.TenantID
	if result != "" {
		return result
	}
	val, ok := a.Ext["tenant_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

func (a *AuthenticationClaims) GetPartitionId() string {

	result := a.PartitionID
	if result != "" {
		return result
	}
	val, ok := a.Ext["partition_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

func (a *AuthenticationClaims) GetAccessId() string {

	result := a.AccessID
	if result != "" {
		return result
	}
	val, ok := a.Ext["access_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

func (a *AuthenticationClaims) GetContactId() string {

	result := a.ContactID
	if result != "" {
		return result
	}
	val, ok := a.Ext["contact_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

func (a *AuthenticationClaims) GetRoles() []string {

	var result = a.Roles
	if len(result) > 0 {
		return result
	}

	roles, ok := a.Ext["roles"]
	if !ok {
		roles, ok = a.Ext["role"]
		if !ok {
			return result
		}
	}

	roleStr, ok2 := roles.(string)
	if ok2 {
		result = append(result, strings.Split(roleStr, ",")...)
	}

	return result
}

func (a *AuthenticationClaims) ServiceName() string {

	result := ""
	val, ok := a.Ext["service_name"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

func (a *AuthenticationClaims) isInternalSystem() bool {

	roles := a.GetRoles()
	if len(roles) == 1 {
		if strings.HasPrefix(roles[0], "system_internal") {
			return true
		}
	}

	return false
}

// AsMetadata Creates a string map to be used as metadata in queue data
func (a *AuthenticationClaims) AsMetadata() map[string]string {

	m := make(map[string]string)
	m["sub"] = a.Subject
	m["tenant_id"] = a.GetTenantId()
	m["partition_id"] = a.GetPartitionId()
	m["access_id"] = a.GetAccessId()
	m["contact_id"] = a.GetContactId()
	m["roles"] = strings.Join(a.GetRoles(), ",")
	return m
}

// ClaimsToContext adds authentication claims to the current supplied context
func (a *AuthenticationClaims) ClaimsToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyAuthenticationClaim, a)
}

// ClaimsFromContext extracts authentication claims from the supplied context if any exist
func ClaimsFromContext(ctx context.Context) *AuthenticationClaims {
	authenticationClaims, ok := ctx.Value(ctxKeyAuthenticationClaim).(*AuthenticationClaims)
	if !ok {
		return nil
	}

	return authenticationClaims
}

// ClaimsFromMap extracts authentication claims from the supplied map if they exist
func ClaimsFromMap(m map[string]string) *AuthenticationClaims {

	authenticationClaims := &AuthenticationClaims{
		Ext: map[string]any{},
	}

	for key, val := range m {
		if key == "sub" {
			authenticationClaims.Subject = m[key]
		} else if key == "tenant_id" {
			authenticationClaims.TenantID = m[key]
		} else if key == "partition_id" {
			authenticationClaims.PartitionID = m[key]
		} else if key == "access_id" {
			authenticationClaims.AccessID = m[key]
		} else if key == "contact_id" {
			authenticationClaims.ContactID = m[key]
		} else if key == "roles" {
			authenticationClaims.Ext[key] = strings.Split(val, ",")
		} else {
			authenticationClaims.Ext[key] = val
		}
	}

	return authenticationClaims
}

func (s *Service) Authenticate(ctx context.Context,
	jwtToken string, audience string, issuer string) (context.Context, error) {
	claims := &AuthenticationClaims{}

	var options []jwt.ParserOption

	if audience != "" {
		options = append(options, jwt.WithAudience(audience))
	}

	if issuer != "" {
		options = append(options, jwt.WithIssuer(issuer))
	}

	token, err := jwt.ParseWithClaims(jwtToken, claims, s.getPemCert, options...)
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

func (s *Service) systemPadPartitionInfo(ctx context.Context, tenantId, partitionId, accessId, contactId, roles string) context.Context {

	claims := ClaimsFromContext(ctx)

	if claims != nil && claims.isInternalSystem() {

		val := claims.GetTenantId()
		if val == "" {
			claims.TenantID = tenantId
		}

		val = claims.GetPartitionId()
		if val == "" {
			claims.PartitionID = partitionId
		}

		val = claims.GetAccessId()
		if val == "" {
			claims.AccessID = accessId
		}

		val = claims.GetContactId()
		if val == "" {
			claims.ContactID = contactId
		}

		valRoles := claims.GetRoles()
		if len(valRoles) == 0 {
			claims.Roles = strings.Split(roles, ",")
		}

		ctx = claims.ClaimsToContext(ctx)
	}

	return ctx
}

type Jwks struct {
	Keys []JSONWebKeys `json:"keys"`
}

type JSONWebKeys struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5c []string `json:"x5c"`
}

func (s *Service) getPemCert(token *jwt.Token) (any, error) {
	config, ok := s.Config().(ConfigurationOAUTH2)
	if !ok {
		return nil, errors.New("could not cast config for oauth2 settings")
	}

	var jwkKeyBytes []byte
	if config.GetOauthWellKnownJwk() == "" {
		return nil, errors.New("web key URL is invalid")
	}

	wellKnownJWK := config.GetOauthWellKnownJwk()

	if strings.HasPrefix(wellKnownJWK, "http") {
		resp, err := http.Get(wellKnownJWK)

		if err != nil {
			return nil, err
		}
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)
		jwkKeyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		wellKnownJWK = string(jwkKeyBytes)
	}

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
			publicKey.E = int(big.NewInt(0).SetBytes(exponent).Uint64())

			// Turn the modulus into a *big.Int.
			publicKey.N = big.NewInt(0).SetBytes(modulus)

			return publicKey, nil
		}
	}

	return nil, errors.New("unable to find appropriate key")
}

// AuthenticationMiddleware Simple http middleware function
// to verify and extract authentication data supplied in a jwt as authorization bearer token
func (s *Service) AuthenticationMiddleware(next http.Handler, audience string, issuer string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")

		logger := s.logger.WithField("authorization_header", authorizationHeader)

		if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, "Bearer ") {
			logger.WithField("available_headers", r.Header).Debug(" AuthenticationMiddleware -- could not authenticate missing token")
			http.Error(w, "An authorization header is required", http.StatusForbidden)
			return
		}

		extractedJwtToken := strings.Split(authorizationHeader, " ")

		if len(extractedJwtToken) != 2 {
			logger.Debug(" AuthenticationMiddleware -- token format is not valid")
			http.Error(w, "Malformed Authorization header", http.StatusBadRequest)
			return
		}

		jwtToken := strings.TrimSpace(extractedJwtToken[1])

		ctx := r.Context()
		ctx, err := s.Authenticate(ctx, jwtToken, audience, issuer)

		if err != nil {
			logger.WithError(err).Info(" AuthenticationMiddleware -- could not authenticate token")
			http.Error(w, "Authorization header is invalid", http.StatusUnauthorized)
			return
		}

		s.systemPadPartitionInfo(ctx, r.Header.Get("tenant_id"), r.Header.Get("partition_id"), r.Header.Get("access_id"), r.Header.Get("contact_id"), r.Header.Get("roles"))

		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

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

// UnaryAuthInterceptor Simple grpc interceptor to extract the jwt supplied via authorization bearer token and verify the authentication claims in the token
func (s *Service) UnaryAuthInterceptor(audience string, issuer string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		jwtToken, err := grpcJwtTokenExtractor(ctx)
		if err != nil {
			return nil, err
		}

		ctx, err = s.Authenticate(ctx, jwtToken, audience, issuer)
		if err != nil {
			logger := s.L().WithError(err).WithField("jwtToken", jwtToken)
			logger.Info(" UnaryAuthInterceptor -- could not authenticate token")
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		ctx = s.systemPadPartitionInfo(ctx, getGrpcMetadata(ctx, "tenant_id"),
			getGrpcMetadata(ctx, "partition_id"), getGrpcMetadata(ctx, "access_id"),
			getGrpcMetadata(ctx, "contact_id"), getGrpcMetadata(ctx, "roles"))

		return handler(ctx, req)
	}
}

// serverStreamWrapper simple wrapper method that stores auth claims for the server stream context
type serverStreamWrapper struct {
	ctx context.Context
	grpc.ServerStream
}

// Context convert the stream wrappers claims to be contained in the stream context
func (s *serverStreamWrapper) Context() context.Context {
	return s.ctx
}

// StreamAuthInterceptor An authentication claims extractor that will always verify the information flowing in the streams as true jwt claims
func (s *Service) StreamAuthInterceptor(audience string, issuer string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		localServerStream := ss
		authClaim := ClaimsFromContext(localServerStream.Context())
		if authClaim == nil {
			ctx := ss.Context()

			jwtToken, err := grpcJwtTokenExtractor(ctx)
			if err != nil {
				return err
			}

			ctx, err = s.Authenticate(ctx, jwtToken, audience, issuer)
			if err != nil {
				logger := s.L().WithError(err).WithField("jwtToken", jwtToken)
				logger.Info(" StreamAuthInterceptor -- could not authenticate token")
				return status.Error(codes.Unauthenticated, err.Error())
			}

			ctx = s.systemPadPartitionInfo(ctx, getGrpcMetadata(ctx, "tenant_id"),
				getGrpcMetadata(ctx, "partition_id"), getGrpcMetadata(ctx, "access_id"),
				getGrpcMetadata(ctx, "contact_id"), getGrpcMetadata(ctx, "roles"))

			localServerStream = &serverStreamWrapper{ctx, ss}
		}
		return handler(srv, localServerStream)
	}
}
