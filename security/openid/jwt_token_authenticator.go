package openid

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

type jwtTokenAuthenticator struct {
	cfg config.ConfigurationOAUTH2
}

func NewJwtTokenAuthenticator(cfg config.ConfigurationOAUTH2) security.Authenticator {
	return &jwtTokenAuthenticator{
		cfg: cfg,
	}
}

func (a *jwtTokenAuthenticator) Authenticate(
	ctx context.Context,
	jwtToken string,
	options ...security.AuthOption,
) (context.Context, error) {
	securityOpts := security.AuthOptions{
		Audience:        a.cfg.GetOauth2ServiceAudience(),
		Issuer:          a.cfg.GetOauth2Issuer(),
		DisableSecurity: false,
	}

	for _, opt := range options {
		opt(ctx, &securityOpts)
	}

	claims := &security.AuthenticationClaims{}

	var parseOptions []jwt.ParserOption

	if securityOpts.Audience != "" {
		parseOptions = append(parseOptions, jwt.WithAudience(securityOpts.Audience))
	}

	if securityOpts.Issuer != "" {
		parseOptions = append(parseOptions, jwt.WithIssuer(securityOpts.Issuer))
	}

	token, err := jwt.ParseWithClaims(jwtToken, claims, a.getPemCert, parseOptions...)
	if err != nil {
		return ctx, err
	}

	if !token.Valid {
		return ctx, errors.New("supplied token was invalid")
	}

	ctx = security.JwtToContext(ctx, jwtToken)

	ctx = claims.ClaimsToContext(ctx)

	return ctx, nil
}

func (a *jwtTokenAuthenticator) getPemCert(token *jwt.Token) (any, error) {
	if a.cfg.GetOauth2WellKnownJwkData() == "" {
		return nil, errors.New("json web key data is not available")
	}

	wellKnownJWK := a.cfg.GetOauth2WellKnownJwkData()

	var jwks = config.Jwks{}
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
