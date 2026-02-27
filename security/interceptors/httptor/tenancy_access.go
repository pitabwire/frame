package httptor

import (
	"net/http"

	"github.com/pitabwire/frame/security/authorizer"
)

// TenancyAccessMiddleware is an HTTP middleware that verifies the caller
// has data access to the partition identified in their claims. It uses
// TenancyAccessChecker.CheckAccess which checks the "member" relation for
// regular users and the "service" relation for system_internal callers.
//
// This middleware should be placed after AuthenticationMiddleware in the
// handler chain so that claims are available in the context.
func TenancyAccessMiddleware(next http.Handler, checker *authorizer.TenancyAccessChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := checker.CheckAccess(r.Context()); err != nil {
			code := authorizer.ToHTTPStatusCode(err)
			http.Error(w, err.Error(), code)
			return
		}

		next.ServeHTTP(w, r)
	})
}
