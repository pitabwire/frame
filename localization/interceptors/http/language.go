package http

import (
	"net/http"

	"github.com/pitabwire/frame/localization"
)

// LanguageHTTPMiddleware is an HTTP middleware that extracts language information and sets it in the context.
func LanguageHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := localization.ExtractLanguageFromHTTPRequest(r)

		ctx := localization.ToContext(r.Context(), l)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
