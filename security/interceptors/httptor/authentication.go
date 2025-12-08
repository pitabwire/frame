package httptor

import (
	"net/http"
	"strings"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/security"
)

const (
	bearerScheme     = "Bearer "
	bearerTokenParts = 2
)

// AuthenticationMiddleware is an HTTP middleware that verifies and extracts authentication
// data supplied in a JWT as an Authorization bearer token.
func AuthenticationMiddleware(
	next http.Handler,
	authenticator security.Authenticator,
	opts ...security.AuthOption,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

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
			next.ServeHTTP(w, r)
			return
		}

		authorizationHeader := r.Header.Get("Authorization")

		logger := util.Log(r.Context()).WithField("authorization_header", authorizationHeader)

		if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, bearerScheme) {
			logger.WithField("available_headers", r.Header).
				Debug(" AuthenticationMiddleware -- could not authenticate missing token")
			http.Error(w, "An authorization header is required", http.StatusUnauthorized)
			return
		}

		extractedJwtToken := strings.Split(authorizationHeader, " ")

		if len(extractedJwtToken) != bearerTokenParts {
			logger.Debug(" AuthenticationMiddleware -- token format is not valid")
			http.Error(w, "Malformed Authorization header", http.StatusBadRequest)
			return
		}

		jwtToken := strings.TrimSpace(extractedJwtToken[1])

		ctx, err := authenticator.Authenticate(ctx, jwtToken, opts...)

		if err != nil {
			logger.WithError(err).Info(" AuthenticationMiddleware -- could not authenticate token")
			http.Error(w, "Authorization header is invalid", http.StatusForbidden)
			return
		}

		security.SetupSecondaryClaims(ctx,
			r.Header.Get("Tenant_id"), r.Header.Get("Partition_id"),
			r.Header.Get("Access_id"), r.Header.Get("Contact_id"),
			r.Header.Get("Session_id"), r.Header.Get("Device_id"),
			r.Header.Get("Roles"))

		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
