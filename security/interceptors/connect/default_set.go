package connect

import (
	"context"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pitabwire/frame/security"
)

func DefaultList(ctx context.Context, authI security.Authenticator, moreInterceptors ...connect.Interceptor) ([]connect.Interceptor, error) {
	var interceptorList []connect.Interceptor

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}

	interceptorList = append(
		moreInterceptors, NewContextSetupInterceptor(ctx),
		otelInterceptor, NewValidationInterceptor(), NewAuthInterceptor(authI))

	return interceptorList, nil
}
