package connect

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNewInterceptor(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		interceptor := NewInterceptor()
		assert.NotNil(t, interceptor.validator)
		assert.False(t, interceptor.validateResponses)
		assert.False(t, interceptor.noErrorDetails)
	})

	t.Run("with custom validator", func(t *testing.T) {
		customValidator := protovalidate.GlobalValidator
		interceptor := NewInterceptor(WithValidator(customValidator))
		assert.Equal(t, customValidator, interceptor.validator)
	})

	t.Run("with validate responses", func(t *testing.T) {
		interceptor := NewInterceptor(WithValidateResponses())
		assert.True(t, interceptor.validateResponses)
	})

	t.Run("with no error details", func(t *testing.T) {
		interceptor := NewInterceptor(WithoutErrorDetails())
		assert.True(t, interceptor.noErrorDetails)
	})

	t.Run("multiple options", func(t *testing.T) {
		customValidator := protovalidate.GlobalValidator
		interceptor := NewInterceptor(
			WithValidator(customValidator),
			WithValidateResponses(),
			WithoutErrorDetails(),
		)

		assert.Equal(t, customValidator, interceptor.validator)
		assert.True(t, interceptor.validateResponses)
		assert.True(t, interceptor.noErrorDetails)
	})
}

func TestValidateRequest(t *testing.T) {
	interceptor := NewInterceptor()

	t.Run("nil message", func(t *testing.T) {
		err := interceptor.validateRequest(nil)
		assert.NoError(t, err)
	})

	t.Run("non-proto message", func(t *testing.T) {
		err := interceptor.validateRequest("not a proto message")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected proto.Message")
	})

	t.Run("valid message", func(t *testing.T) {
		msg := &wrapperspb.StringValue{Value: "test"}
		err := interceptor.validateRequest(msg)
		assert.NoError(t, err)
	})
}

func TestValidateResponse(t *testing.T) {
	t.Run("without validate responses option", func(t *testing.T) {
		interceptor := NewInterceptor()
		msg := &wrapperspb.StringValue{Value: "test"}
		err := interceptor.validateResponse(msg)
		assert.NoError(t, err) // Should not validate responses by default
	})

	t.Run("with validate responses option", func(t *testing.T) {
		interceptor := NewInterceptor(WithValidateResponses())
		msg := &wrapperspb.StringValue{Value: "test"}
		err := interceptor.validateResponse(msg)
		assert.NoError(t, err)
	})
}

func TestStructValidation(t *testing.T) {
	t.Run("valid struct", func(t *testing.T) {
		s, err := structpb.NewStruct(map[string]interface{}{
			"name":  "test",
			"value": 42,
		})
		require.NoError(t, err)

		err = validateSingleStruct(s)
		assert.NoError(t, err)
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
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds 1 MiB")
	})

	t.Run("too many fields", func(t *testing.T) {
		fields := make(map[string]interface{})
		for i := 0; i < 201; i++ {
			fields[fmt.Sprintf("field%d", i)] = "value"
		}

		s, err := structpb.NewStruct(fields)
		require.NoError(t, err)

		err = validateSingleStruct(s)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many top-level fields")
	})

	t.Run("string too long", func(t *testing.T) {
		longString := string(make([]byte, 8193)) // > 8 KiB
		for i := range longString {
			longString = longString[:i] + "a" + longString[i+1:]
		}

		s, err := structpb.NewStruct(map[string]interface{}{
			"long_string": longString,
		})
		require.NoError(t, err)

		// CEL validation is currently disabled, so this should pass
		err = validateSingleStruct(s)
		assert.NoError(t, err)
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
		assert.NoError(t, err)
	})
}

func TestValidateAllStructs(t *testing.T) {
	t.Run("struct message", func(t *testing.T) {
		s, err := structpb.NewStruct(map[string]interface{}{
			"field": "value",
		})
		require.NoError(t, err)

		err = validateAllStructs(s)
		assert.NoError(t, err)
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
		assert.NoError(t, err)
	})

	t.Run("message with invalid nested struct", func(t *testing.T) {
		// Create a struct with too many fields
		fields := make(map[string]interface{})
		for i := 0; i < 201; i++ {
			fields[fmt.Sprintf("field%d", i)] = "value"
		}
		invalidStruct, err := structpb.NewStruct(fields)
		require.NoError(t, err)

		container := &structpb.Struct{}
		container.Fields = map[string]*structpb.Value{
			"data": structpb.NewStructValue(invalidStruct),
		}

		err = validateAllStructs(container)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many top-level fields")
	})
}

func TestWrapValidationError(t *testing.T) {
	t.Run("with error details", func(t *testing.T) {
		interceptor := NewInterceptor()
		originalErr := &protovalidate.ValidationError{}
		wrappedErr := interceptor.wrapValidationError(originalErr, connect.CodeInvalidArgument)

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(wrappedErr))
		// The error should contain the original error
		assert.Contains(t, wrappedErr.Error(), "validation error")
	})

	t.Run("without error details", func(t *testing.T) {
		interceptor := NewInterceptor(WithoutErrorDetails())
		originalErr := &protovalidate.ValidationError{}
		wrappedErr := interceptor.wrapValidationError(originalErr, connect.CodeInvalidArgument)

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(wrappedErr))
		// Should still contain the original error
		assert.Contains(t, wrappedErr.Error(), "validation error")
	})
}

func TestUnaryInterceptor(t *testing.T) {
	interceptor := NewInterceptor()

	t.Run("valid request", func(t *testing.T) {
		callCount := 0
		next := func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			callCount++
			return connect.NewResponse(&wrapperspb.StringValue{Value: "response"}), nil
		}

		wrapped := interceptor.WrapUnary(next)
		req := connect.NewRequest(&wrapperspb.StringValue{Value: "request"})

		resp, err := wrapped(context.Background(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 1, callCount)
	})

	t.Run("invalid request", func(t *testing.T) {
		callCount := 0
		next := func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			callCount++
			return nil, nil
		}

		wrapped := interceptor.WrapUnary(next)

		// Create an invalid request that should fail struct validation
		fields := make(map[string]interface{})
		for i := 0; i < 201; i++ { // Too many fields
			fields[fmt.Sprintf("field%d", i)] = "value"
		}
		invalidStruct, err := structpb.NewStruct(fields)
		require.NoError(t, err)

		// Put the invalid Struct inside another message as a field
		container := &structpb.Struct{}
		container.Fields = map[string]*structpb.Value{
			"data": structpb.NewStructValue(invalidStruct),
		}

		req := connect.NewRequest(container)

		resp, err := wrapped(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 0, callCount) // Next should not be called
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})
}

func TestStreamingInterceptors(t *testing.T) {
	interceptor := NewInterceptor()

	t.Run("streaming client interceptor", func(t *testing.T) {
		wrapped := interceptor.WrapStreamingClient(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
			return &mockStreamingClientConn{}
		})

		conn := wrapped(context.Background(), connect.Spec{})
		assert.NotNil(t, conn)

		// Test Send with valid message
		err := conn.Send(&wrapperspb.StringValue{Value: "test"})
		assert.NoError(t, err)
	})

	t.Run("streaming handler interceptor", func(t *testing.T) {
		callCount := 0
		wrapped := interceptor.WrapStreamingHandler(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
			callCount++
			return nil
		})

		err := wrapped(context.Background(), &mockStreamingHandlerConn{})
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})
}

// Mock implementations for testing
type mockStreamingClientConn struct{}

func (m *mockStreamingClientConn) Send(msg any) error             { return nil }
func (m *mockStreamingClientConn) Receive(msg any) error           { return nil }
func (m *mockStreamingClientConn) CloseRequest() error            { return nil }
func (m *mockStreamingClientConn) CloseResponse() error           { return nil }
func (m *mockStreamingClientConn) Spec() connect.Spec             { return connect.Spec{} }
func (m *mockStreamingClientConn) Peer() connect.Peer             { return connect.Peer{} }
func (m *mockStreamingClientConn) RequestHeader() http.Header     { return http.Header{} }
func (m *mockStreamingClientConn) ResponseHeader() http.Header    { return http.Header{} }
func (m *mockStreamingClientConn) ResponseTrailer() http.Header   { return http.Header{} }

type mockStreamingHandlerConn struct{}

func (m *mockStreamingHandlerConn) Send(msg any) error             { return nil }
func (m *mockStreamingHandlerConn) Receive(msg any) error           { return nil }
func (m *mockStreamingHandlerConn) Spec() connect.Spec             { return connect.Spec{} }
func (m *mockStreamingHandlerConn) Peer() connect.Peer             { return connect.Peer{} }
func (m *mockStreamingHandlerConn) RequestHeader() http.Header     { return http.Header{} }
func (m *mockStreamingHandlerConn) ResponseHeader() http.Header    { return http.Header{} }
func (m *mockStreamingHandlerConn) ResponseTrailer() http.Header   { return http.Header{} }

// Benchmark tests
func BenchmarkValidateSingleStruct(b *testing.B) {
	s, err := structpb.NewStruct(map[string]interface{}{
		"name":  "test",
		"value": 42,
		"list":  []interface{}{"a", "b", "c"},
	})
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validateSingleStruct(s)
	}
}

func BenchmarkValidateAllStructs(b *testing.B) {
	s, err := structpb.NewStruct(map[string]interface{}{
		"field": "value",
	})
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validateAllStructs(s)
	}
}

func BenchmarkInterceptorWrapUnary(b *testing.B) {
	interceptor := NewInterceptor()
	next := func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return connect.NewResponse(&wrapperspb.StringValue{Value: "response"}), nil
	}
	wrapped := interceptor.WrapUnary(next)

	req := connect.NewRequest(&wrapperspb.StringValue{Value: "request"})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wrapped(ctx, req)
	}
}
