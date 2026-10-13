package connect

import (
	"context"
	"net/http"

	"connectrpc.com/connect/v2"

	"github.com/pitabwire/frame/v3/localization"
)

const acceptLanguageHeader = "Accept-Language"

// NewLanguageInterceptor returns a Connect server interceptor that reads the
// Accept-Language request header and stores the requested languages in the
// context passed to the handler, for unary and streaming RPCs alike.
func NewLanguageInterceptor() (connect.ServerInterceptor, error) {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if info, ok := connect.CallInfoForServerContext(ctx); ok {
				header := http.Header{acceptLanguageHeader: info.RequestHeader().Values(acceptLanguageHeader)}
				ctx = localization.ToContext(ctx, localization.ExtractLanguageFromHTTPHeader(header))
			}
			return next(ctx, spec, stream)
		}
	}, nil
}
