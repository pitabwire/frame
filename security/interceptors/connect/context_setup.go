package connect

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
)

type ctxSetupInterceptor struct {
	cfg         any
	logger      *util.LogEntry
	propagators []func(ctx context.Context) context.Context
}

// NewContextSetupInterceptor creates a new context propagation interceptor.
func NewContextSetupInterceptor(
	mainCtx context.Context,
	propagators ...func(ctx context.Context) context.Context,
) connect.Interceptor {
	return &ctxSetupInterceptor{
		cfg:         config.FromContext[any](mainCtx),
		logger:      util.Log(mainCtx),
		propagators: propagators,
	}
}

func (c *ctxSetupInterceptor) propagate(ctx context.Context) context.Context {
	reqCtx := util.ContextWithLogger(ctx, c.logger)
	if c.cfg != nil {
		reqCtx = config.ToContext(reqCtx, c.cfg)
	}

	for _, pi := range c.propagators {
		reqCtx = pi(reqCtx)
	}
	return reqCtx
}

func (c *ctxSetupInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return next(c.propagate(ctx), req)
	}
}

func (c *ctxSetupInterceptor) WrapStreamingClient(
	clientFunc connect.StreamingClientFunc,
) connect.StreamingClientFunc {
	return clientFunc
}

func (c *ctxSetupInterceptor) WrapStreamingHandler(
	handlerFunc connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return handlerFunc(c.propagate(ctx), conn)
	}
}
