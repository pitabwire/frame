//nolint:testpackage // Internal package testing shares the in-process echo helpers.
package connect

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"github.com/pitabwire/frame/v3/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

const subjectProcedure = "/frame.test.v1.EchoService/Subject"

type stubAuthenticator struct {
	token string
}

func (a stubAuthenticator) Authenticate(
	ctx context.Context, jwtToken string, _ ...security.AuthOption,
) (context.Context, error) {
	if jwtToken != a.token {
		return nil, errors.New("bad token")
	}
	claims := &security.AuthenticationClaims{TenantID: "T1", PartitionID: "P1"}
	claims.Subject = "user-1"
	return claims.ClaimsToContext(ctx), nil
}

// newSubjectClient serves a unary method that returns the authenticated
// subject seen by the handler.
func newSubjectClient(t *testing.T, interceptors ...connect.ServerInterceptor) *connect.Client {
	t.Helper()
	server := connect.NewServer(interceptors...)
	server.Register(connect.Method{
		Spec: connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: subjectProcedure},
		Handler: func(ctx context.Context, _ connect.Spec, stream connect.ServerStream) error {
			if err := stream.Receive(&structpb.Struct{}); err != nil {
				return err
			}
			subject := ""
			if claims := security.ClaimsFromContext(ctx); claims != nil {
				subject = claims.Subject
			}
			res, err := structpb.NewStruct(map[string]any{"subject": subject})
			if err != nil {
				return err
			}
			return stream.Send(res)
		},
	})
	return connect.NewClient(connectinprocess.New(server))
}

func TestAuthInterceptor(t *testing.T) {
	client := newSubjectClient(t, NewAuthInterceptor(stubAuthenticator{token: "good"}))
	spec := connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: subjectProcedure}

	tests := []struct {
		name        string
		header      string
		wantCode    connect.Code
		wantSubject string
	}{
		{name: "valid bearer token", header: "Bearer good", wantSubject: "user-1"},
		{name: "missing header", wantCode: connect.CodeUnauthenticated},
		{name: "not a bearer token", header: "Basic abc", wantCode: connect.CodeUnauthenticated},
		{name: "empty bearer token", header: "Bearer  ", wantCode: connect.CodeUnauthenticated},
		{name: "rejected token", header: "Bearer bad", wantCode: connect.CodeUnauthenticated},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, info := connect.NewClientContext(t.Context())
			if tc.header != "" {
				info.RequestHeader().Set(authorizationKey, tc.header)
			}

			res := &structpb.Struct{}
			err := client.CallUnary(ctx, spec, &structpb.Struct{}, res)
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, connect.CodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantSubject, res.GetFields()["subject"].GetStringValue())
		})
	}
}

func TestDefaultListAuthenticatesAndValidates(t *testing.T) {
	interceptors, err := DefaultList(t.Context(), stubAuthenticator{token: "good"})
	require.NoError(t, err)
	client := newSubjectClient(t, interceptors...)
	spec := connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: subjectProcedure}

	ctx, info := connect.NewClientContext(t.Context())
	info.RequestHeader().Set(authorizationKey, "Bearer good")
	res := &structpb.Struct{}
	require.NoError(t, client.CallUnary(ctx, spec, &structpb.Struct{}, res))
	assert.Equal(t, "user-1", res.GetFields()["subject"].GetStringValue())

	err = client.CallUnary(t.Context(), spec, &structpb.Struct{}, &structpb.Struct{})
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
