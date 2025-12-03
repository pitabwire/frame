package http

import (
	"context"
	"net/http"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
)

// ContextSetupMiddleware propagates logger in main context into HTTP context.
func ContextSetupMiddleware(
	mainCtx context.Context,
	next http.Handler,
	propagators ...func(ctx context.Context) context.Context,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCtx := r.Context()
		cfg := config.FromContext[any](mainCtx)
		if cfg != nil {
			reqCtx = config.ToContext(reqCtx, cfg)
		}
		logger := util.Log(mainCtx)
		reqCtx = util.ContextWithLogger(reqCtx, logger)

		for _, pi := range propagators {
			reqCtx = pi(reqCtx)
		}

		// Replace the request with the merged context
		r = r.WithContext(reqCtx)

		next.ServeHTTP(w, r)
	})
}
