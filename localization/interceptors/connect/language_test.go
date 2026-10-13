package connect_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"github.com/pitabwire/frame/v3/localization"
	languageix "github.com/pitabwire/frame/v3/localization/interceptors/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestLanguageInterceptor(t *testing.T) {
	interceptor, err := languageix.NewLanguageInterceptor()
	require.NoError(t, err)

	spec := connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: "/frame.test.v1.Lang/Get"}
	server := connect.NewServer(interceptor)
	server.Register(connect.Method{
		Spec: spec,
		Handler: func(ctx context.Context, _ connect.Spec, stream connect.ServerStream) error {
			if recvErr := stream.Receive(&wrapperspb.StringValue{}); recvErr != nil {
				return recvErr
			}
			return stream.Send(wrapperspb.String(strings.Join(localization.FromContext(ctx), "|")))
		},
	})
	client := connect.NewClient(connectinprocess.New(server))

	ctx, info := connect.NewClientContext(t.Context())
	info.RequestHeader().Set("Accept-Language", "sw,en")
	res := &wrapperspb.StringValue{}
	require.NoError(t, client.CallUnary(ctx, spec, wrapperspb.String(""), res))
	require.Equal(t, "sw|en", res.GetValue())
}
