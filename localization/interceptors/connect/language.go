package connect

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pitabwire/frame/localization"
)

// LanguageInterceptor implements connect.Interceptor for ensuring language is available in the context.
type LanguageInterceptor struct {
}

// NewLanguageInterceptor creates a new validation interceptor with default options.
func NewLanguageInterceptor() (*LanguageInterceptor, error) {
	return &LanguageInterceptor{}, nil
}

// WrapUnary validates unary requests and responses.
func (l *LanguageInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		l := localization.ExtractLanguageFromHTTPHeader(req.Header())

		ctx = localization.ToContext(ctx, l)

		// Call the handler
		return next(ctx, req)
	}
}

// WrapStreamingClient validates streaming client messages (pass-through for server-side).
func (l *LanguageInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler validates streaming messages.
func (l *LanguageInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		// Wrap the connection to intercept Receive and Send calls
		wrappedConn := &languageStreamConn{
			StreamingHandlerConn: conn,
			interceptor:          l,
			ctx:                  ctx,
		}

		return next(ctx, wrappedConn)
	}
}

// languageStreamConn wraps a StreamingHandlerConn to validate messages.
type languageStreamConn struct {
	connect.StreamingHandlerConn
	interceptor *LanguageInterceptor
	ctx         context.Context
}

// Receive validates incoming stream messages.
func (v *languageStreamConn) Receive(msg any) error {
	l := localization.ExtractLanguageFromHTTPHeader(v.StreamingHandlerConn.RequestHeader())

	v.ctx = localization.ToContext(v.ctx, l)

	if err := v.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}

	return nil
}

// Send validates outgoing stream messages.
func (v *languageStreamConn) Send(msg any) error {
	return v.StreamingHandlerConn.Send(msg)
}
