package connect

import (
	"context"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pitabwire/frame/security"
)

func DefaultInterceptorList(ctx context.Context, authI security.Authenticator) ([]connect.Interceptor, error) {
	var interceptorList []connect.Interceptor

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}

	interceptorList = append(
		interceptorList, NewContextSetupInterceptor(ctx),
		otelInterceptor, NewValidationInterceptor(), NewAuthInterceptor(authI))

	return interceptorList, nil
}
