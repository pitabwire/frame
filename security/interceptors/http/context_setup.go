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
	ctxFnList ...func(ctx context.Context) context.Context,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCtx := r.Context()
		cfg := config.FromContext[any](mainCtx)
		if cfg != nil {
			reqCtx = config.ToContext(reqCtx, cfg)
		}
		logger := util.Log(mainCtx)
		reqCtx = util.ContextWithLogger(reqCtx, logger)

		if len(ctxFnList) > 0 {
			ctxFn := ctxFnList[0]
			ctx := ctxFn(reqCtx)
			if ctx != nil {
				reqCtx = ctx
			}
		}

		r = r.WithContext(reqCtx)

		next.ServeHTTP(w, r)
	})
}
