package httptor

import (
	"net/http"

	"github.com/pitabwire/frame/security/authorizer"
)

// FunctionAccessMiddleware is an HTTP middleware that enforces functional
// permissions based on a mapping of HTTP path patterns to required permissions.
//
// The permissions map keys are HTTP path prefixes (e.g., "/api/profiles") and
// values are slices of permission strings that must ALL be satisfied (AND logic).
//
// If a request path matches multiple prefixes, the longest matching prefix is used.
// If no prefix matches, the request is allowed through without a permission check.
//
// This middleware should be placed after AuthenticationMiddleware and
// TenancyAccessMiddleware in the handler chain.
func FunctionAccessMiddleware(
	next http.Handler,
	checker *authorizer.FunctionChecker,
	permissions map[string][]string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perms := matchPermissions(r.URL.Path, permissions)
		if len(perms) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		for _, perm := range perms {
			if err := checker.Check(r.Context(), perm); err != nil {
				code := authorizer.ToHTTPStatusCode(err)
				http.Error(w, err.Error(), code)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// matchPermissions finds the longest matching path prefix in the permissions map.
func matchPermissions(path string, permissions map[string][]string) []string {
	var bestMatch string
	var bestPerms []string

	for prefix, perms := range permissions {
		if len(prefix) > len(bestMatch) && hasPrefix(path, prefix) {
			bestMatch = prefix
			bestPerms = perms
		}
	}

	return bestPerms
}

func hasPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	if path[:len(prefix)] != prefix {
		return false
	}
	// Exact match, prefix ends with '/', or the next character is a path separator.
	// This prevents "/api/profile" from matching "/api/profileadmin".
	return len(path) == len(prefix) || prefix[len(prefix)-1] == '/' || path[len(prefix)] == '/'
}
