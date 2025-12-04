package grpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/pitabwire/frame/localization"
)

// LanguageUnaryInterceptor Simple grpc interceptor to extract the language supplied via metadata.
func LanguageUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		_ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		l := localization.ExtractLanguageFromGrpcRequest(ctx)
		if l != nil {
			ctx = localization.ToContext(ctx, l)
		}

		return handler(ctx, req)
	}
}

// LanguageStreamInterceptor A language extractor that will extract .
func LanguageStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		l := localization.ExtractLanguageFromGrpcRequest(ctx)
		if l == nil {
			return handler(svc, ss)
		}

		ctx = localization.ToContext(ctx, l)

		// Wrap the original stream with ctx this ensures the handlers always receives a stream from which it can get the correct context.
		languageStream := &serverStreamWrapper{ctx, ss}

		return handler(svc, languageStream)
	}
}

// serverStreamWrapper simple wrapper method that stores auth claims for the server stream context.
type serverStreamWrapper struct {
	ctx context.Context
	grpc.ServerStream
}

// Context converts the stream wrappers claims to be contained in the stream context.
func (s *serverStreamWrapper) Context() context.Context {
	return s.ctx
}
