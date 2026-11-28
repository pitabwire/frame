package grpc_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/pitabwire/frame/v3/security"
	grpctor "github.com/pitabwire/frame/v3/security/interceptors/grpc"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type rejectingAuthenticator struct{}

func (rejectingAuthenticator) Authenticate(
	_ context.Context, _ string, _ ...security.AuthOption,
) (context.Context, error) {
	return nil, errors.New("token is expired")
}

type stubServerStream struct {
	grpclib.ServerStream
	ctx context.Context
}

func (s *stubServerStream) Context() context.Context { return s.ctx }

func TestAuthInterceptors_NeverLogCredentials(t *testing.T) {
	const secretToken = "eyJhbGciOiJSUzI1NiJ9.Z3JwYy1zZWNyZXQ.c2lnLXNlY3JldA"

	cases := []struct {
		name string
		call func(ctx context.Context) error
	}{
		{
			name: "unary",
			call: func(ctx context.Context) error {
				_, err := grpctor.UnaryAuthInterceptor(rejectingAuthenticator{})(ctx, nil,
					&grpclib.UnaryServerInfo{FullMethod: "/svc/Method"},
					func(context.Context, any) (any, error) {
						t.Fatal("handler must not run for a rejected token")
						return struct{}{}, nil
					})
				return err
			},
		},
		{
			name: "stream",
			call: func(ctx context.Context) error {
				return grpctor.StreamAuthInterceptor(rejectingAuthenticator{})(nil,
					&stubServerStream{ctx: ctx},
					&grpclib.StreamServerInfo{FullMethod: "/svc/Stream"},
					func(any, grpclib.ServerStream) error {
						t.Fatal("handler must not run for a rejected token")
						return nil
					})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := util.NewLogger(t.Context(),
				util.WithLogOutput(buf),
				util.WithLogLevel(slog.LevelDebug),
				util.WithLogNoColor(true),
				util.WithLogHandlerExclusive(),
			)
			ctx := util.ContextWithLogger(t.Context(), logger)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+secretToken))

			err := tc.call(ctx)

			require.Error(t, err)
			require.NotContains(t, buf.String(), secretToken, "credentials must never reach the logs")
			require.Contains(t, buf.String(), security.TokenFingerprint(secretToken))
		})
	}
}
