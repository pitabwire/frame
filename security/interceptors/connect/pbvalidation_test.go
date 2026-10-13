//nolint:testpackage // Internal package testing requires same package name
package connect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStructValidation(t *testing.T) {
	t.Run("valid struct", func(t *testing.T) {
		s, err := structpb.NewStruct(map[string]interface{}{
			"name":  "test",
			"value": 42,
		})
		require.NoError(t, err)

		err = validateSingleStruct(s)
		require.NoError(t, err)
	})

	t.Run("struct too large", func(t *testing.T) {
		// Create a struct larger than 1 MiB
		largeData := make([]byte, 1024*1024+1)
		for i := range largeData {
			largeData[i] = 'a'
		}

		s, err := structpb.NewStruct(map[string]interface{}{
			"large_field": string(largeData),
		})
		require.NoError(t, err)

		err = validateSingleStruct(s)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds 1 MiB")
	})

	t.Run("too many fields", func(t *testing.T) {
		fields := make(map[string]interface{})
		for i := range 201 {
			fields[fmt.Sprintf("field%d", i)] = "value"
		}

		s, err := structpb.NewStruct(fields)
		require.NoError(t, err)

		err = validateSingleStruct(s)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many top-level fields")
	})

	t.Run("string too long", func(t *testing.T) {
		longBytes := make([]byte, 8193) // > 8 KiB
		for i := range longBytes {
			longBytes[i] = 'a'
		}
		longString := string(longBytes)

		s, err := structpb.NewStruct(map[string]interface{}{
			"long_string": longString,
		})
		require.NoError(t, err)

		// CEL validation is currently disabled, so this should pass
		err = validateSingleStruct(s)
		require.NoError(t, err)
	})

	t.Run("list too long", func(t *testing.T) {
		longList := make([]interface{}, 5001) // > 5000 elements
		for i := range longList {
			longList[i] = "item"
		}

		s, err := structpb.NewStruct(map[string]interface{}{
			"long_list": longList,
		})
		require.NoError(t, err)

		// CEL validation is currently disabled, so this should pass
		err = validateSingleStruct(s)
		require.NoError(t, err)
	})
}

func TestValidateAllStructs(t *testing.T) {
	t.Run("struct message", func(t *testing.T) {
		s, err := structpb.NewStruct(map[string]interface{}{
			"field": "value",
		})
		require.NoError(t, err)

		err = validateAllStructs(s)
		require.NoError(t, err)
	})

	t.Run("message with nested struct", func(t *testing.T) {
		// Create a message that contains a struct in a list
		nestedStruct, err := structpb.NewStruct(map[string]interface{}{
			"nested_field": "value",
		})
		require.NoError(t, err)

		container := &structpb.Struct{}
		container.Fields = map[string]*structpb.Value{
			"data": structpb.NewStructValue(nestedStruct),
		}

		err = validateAllStructs(container)
		require.NoError(t, err)
	})

	t.Run("message with deeply nested invalid struct", func(t *testing.T) {
		// Create a deeply nested struct where an inner struct has too many fields
		innerInvalidStruct := &structpb.Struct{}
		innerInvalidStruct.Fields = make(map[string]*structpb.Value)
		for i := range 201 { // Too many fields
			innerInvalidStruct.Fields[fmt.Sprintf("field%d", i)] = structpb.NewStringValue("value")
		}

		// Create a middle struct that contains the invalid inner struct
		middleStruct := &structpb.Struct{}
		middleStruct.Fields = map[string]*structpb.Value{
			"invalid_nested": structpb.NewStructValue(innerInvalidStruct),
			"valid_field":    structpb.NewStringValue("ok"),
		}

		// Create the top-level container
		container := &structpb.Struct{}
		container.Fields = map[string]*structpb.Value{
			"data": structpb.NewStructValue(middleStruct),
		}

		err := validateAllStructs(container)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many top-level fields")
	})
}

const (
	echoProcedure       = "/frame.test.v1.EchoService/Echo"
	echoStreamProcedure = "/frame.test.v1.EchoService/EchoStream"
)

// newEchoClient registers unary and bidi echo methods for structpb.Struct on a
// server built with the given server interceptors, and returns an in-process
// client for it plus a counter of handler invocations.
func newEchoClient(
	t testing.TB,
	serverInterceptors []connect.ServerInterceptor,
	clientInterceptors ...connect.ClientInterceptor,
) (*connect.Client, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	server := connect.NewServer(serverInterceptors...)
	server.Register(
		connect.Method{
			Spec: connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: echoProcedure},
			Handler: func(_ context.Context, _ connect.Spec, stream connect.ServerStream) error {
				calls.Add(1)
				msg := &structpb.Struct{}
				if err := stream.Receive(msg); err != nil {
					return err
				}
				return stream.Send(msg)
			},
		},
		connect.Method{
			Spec: connect.Spec{StreamType: connect.StreamTypeBidi, Procedure: echoStreamProcedure},
			Handler: func(_ context.Context, _ connect.Spec, stream connect.ServerStream) error {
				calls.Add(1)
				for {
					msg := &structpb.Struct{}
					if err := stream.Receive(msg); err != nil {
						if errors.Is(err, io.EOF) {
							return nil
						}
						return err
					}
					if err := stream.Send(msg); err != nil {
						return err
					}
				}
			},
		},
	)

	return connect.NewClient(connectinprocess.New(server), clientInterceptors...), &calls
}

func tooManyFieldsStruct(t testing.TB) *structpb.Struct {
	t.Helper()
	fields := make(map[string]interface{})
	for i := range 201 {
		fields[fmt.Sprintf("field%d", i)] = "value"
	}
	invalidStruct, err := structpb.NewStruct(fields)
	require.NoError(t, err)

	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"data": structpb.NewStructValue(invalidStruct),
	}}
}

func TestServerInterceptorUnary(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		client, calls := newEchoClient(t, []connect.ServerInterceptor{NewValidationInterceptor()})
		req, err := structpb.NewStruct(map[string]interface{}{"name": "frame"})
		require.NoError(t, err)

		res := &structpb.Struct{}
		err = client.CallUnary(t.Context(), connect.Spec{
			StreamType: connect.StreamTypeUnary, Procedure: echoProcedure,
		}, req, res)
		require.NoError(t, err)
		assert.Equal(t, "frame", res.GetFields()["name"].GetStringValue())
		assert.Equal(t, int32(1), calls.Load())
	})

	t.Run("invalid request", func(t *testing.T) {
		client, _ := newEchoClient(t, []connect.ServerInterceptor{NewValidationInterceptor()})

		err := client.CallUnary(t.Context(), connect.Spec{
			StreamType: connect.StreamTypeUnary, Procedure: echoProcedure,
		}, tooManyFieldsStruct(t), &structpb.Struct{})
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "too many top-level fields")
	})
}

func TestServerInterceptorStreaming(t *testing.T) {
	client, calls := newEchoClient(t, []connect.ServerInterceptor{NewValidationInterceptor()})
	spec := connect.Spec{StreamType: connect.StreamTypeBidi, Procedure: echoStreamProcedure}

	t.Run("valid messages pass", func(t *testing.T) {
		stream, err := client.CallClientStream(t.Context(), spec)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }()

		msg, err := structpb.NewStruct(map[string]interface{}{"n": 1})
		require.NoError(t, err)
		require.NoError(t, stream.Send(msg))
		require.NoError(t, stream.CloseSend())

		got := &structpb.Struct{}
		require.NoError(t, stream.Receive(got))
		assert.InDelta(t, 1.0, got.GetFields()["n"].GetNumberValue(), 0)
		require.ErrorIs(t, stream.Receive(&structpb.Struct{}), io.EOF)
		assert.Positive(t, calls.Load())
	})

	t.Run("invalid message fails the stream", func(t *testing.T) {
		stream, err := client.CallClientStream(t.Context(), spec)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }()

		require.NoError(t, stream.Send(tooManyFieldsStruct(t)))
		require.NoError(t, stream.CloseSend())

		err = stream.Receive(&structpb.Struct{})
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})
}

func TestClientInterceptor(t *testing.T) {
	client, calls := newEchoClient(t, nil, NewValidationClientInterceptor())

	err := client.CallUnary(t.Context(), connect.Spec{
		StreamType: connect.StreamTypeUnary, Procedure: echoProcedure,
	}, tooManyFieldsStruct(t), &structpb.Struct{})
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Equal(t, int32(0), calls.Load(), "an invalid request must not reach the server")
}

func TestValidationErrorDetails(t *testing.T) {
	i := newValidationInterceptor()
	err := i.validateRequest(&wrapperspb.StringValue{Value: "ok"})
	require.NoError(t, err)

	err = i.validateRequest("not a proto message")
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	noDetails := newValidationInterceptor(WithoutErrorDetails(), WithValidateResponses())
	err = noDetails.validateResponse(tooManyFieldsStruct(t))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

// Benchmark tests.
func BenchmarkValidateSingleStruct(b *testing.B) {
	s, err := structpb.NewStruct(map[string]interface{}{
		"name":  "test",
		"value": 42,
		"list":  []interface{}{"a", "b", "c"},
	})
	require.NoError(b, err)

	b.ResetTimer()
	for range b.N {
		_ = validateSingleStruct(s)
	}
}

func BenchmarkValidateAllStructs(b *testing.B) {
	s, err := structpb.NewStruct(map[string]interface{}{
		"field": "value",
	})
	require.NoError(b, err)

	b.ResetTimer()
	for range b.N {
		_ = validateAllStructs(s)
	}
}

func BenchmarkServerInterceptorUnary(b *testing.B) {
	client, _ := newEchoClient(b, []connect.ServerInterceptor{NewValidationInterceptor()})
	spec := connect.Spec{StreamType: connect.StreamTypeUnary, Procedure: echoProcedure}
	req, err := structpb.NewStruct(map[string]interface{}{"name": "frame"})
	require.NoError(b, err)
	ctx := b.Context()

	b.ResetTimer()
	for range b.N {
		_ = client.CallUnary(ctx, spec, req, &structpb.Struct{})
	}
}
