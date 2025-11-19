package connect

import (
	"context"
	"errors"
	"fmt"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// maxStructSizeB is the maximum allowed size in bytes for a single StructPB message (1 MiB).
	maxStructSizeB = 1024 * 1024
	// maxStructFields is the maximum allowed number of top-level fields in a StructPB message.
	maxStructFields = 200
)

// An Option configures an [ValidationInterceptor].
type Option interface {
	apply(*ValidationInterceptor)
}

// WithValidator configures the [ValidationInterceptor] to use a customized
// [protovalidate.Validator]. By default, [protovalidate.GlobalInterceptor]
// is used See [protovalidate.ValidatorOption] for the range of available
// customizations.
func WithValidator(validator protovalidate.Validator) Option {
	return optionFunc(func(i *ValidationInterceptor) {
		i.validator = validator
	})
}

// WithValidateResponses configures the [ValidationInterceptor] to also validate reponses
// in addition to validating requests.
//
// By default:
//
// - Unary: Response messages from the server are not validated.
// - Client streams: Received messages are not validated.
// - Server streams: Sent messages are not validated.
//
// However, these messages are all validated if this option is set.
func WithValidateResponses() Option {
	return optionFunc(func(i *ValidationInterceptor) {
		i.validateResponses = true
	})
}

// WithoutErrorDetails configures the [ValidationInterceptor] to elide error details from
// validation errors. By default, a [protovalidate.ValidationError] is added
// as a detail when validation errors are returned.
func WithoutErrorDetails() Option {
	return optionFunc(func(i *ValidationInterceptor) {
		i.noErrorDetails = true
	})
}

// ValidationInterceptor is a [connect.Interceptor] that ensures that RPC request
// messages match the constraints expressed in their Protobuf schemas. It does
// not validate response messages unless the [WithValidateResponses] option
// is specified.
//
// By default, Interceptors use a validator that lazily compiles constraints
// and works with any Protobuf message. This is a simple, widely-applicable
// configuration: after compiling and caching the constraints for a Protobuf
// message type once, validation is very efficient. To customize the validator,
// use [WithValidator] and [protovalidate.ValidatorOption].
//
// RPCs with invalid request messages short-circuit with an error. The error
// always uses [connect.CodeInvalidArgument] and has a [detailed representation
// of the error] attached as a [connect.ErrorDetail].
//
// This interceptor is primarily intended for use on handlers. Client-side use
// is possible, but discouraged unless the client always has an up-to-date
// schema.
//
// [detailed representation of the error]: https://pkg.go.dev/buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate#Violations
type ValidationInterceptor struct {
	validator         protovalidate.Validator
	validateResponses bool
	noErrorDetails    bool
}

// NewValidationInterceptor builds an ValidationInterceptor. The default configuration is
// appropriate for most use cases.
func NewValidationInterceptor(opts ...Option) *ValidationInterceptor {
	var interceptor ValidationInterceptor
	for _, opt := range opts {
		opt.apply(&interceptor)
	}

	if interceptor.validator == nil {
		interceptor.validator = protovalidate.GlobalValidator
	}

	return &interceptor
}

// WrapUnary implements connect.ValidationInterceptor.
func (i *ValidationInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.validateRequest(req.Any()); err != nil {
			return nil, err
		}
		response, err := next(ctx, req)
		if err != nil {
			return response, err
		}
		if validationErr := i.validateResponse(response.Any()); validationErr != nil {
			return response, validationErr
		}
		return response, nil
	}
}

// WrapStreamingClient implements connect.ValidationInterceptor.
func (i *ValidationInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return &streamingClientInterceptor{
			StreamingClientConn: next(ctx, spec),
			interceptor:         i,
		}
	}
}

// WrapStreamingHandler implements connect.ValidationInterceptor.
func (i *ValidationInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, &streamingHandlerInterceptor{
			StreamingHandlerConn: conn,
			interceptor:          i,
		})
	}
}

func (i *ValidationInterceptor) validateRequest(msg any) error {
	return i.validate(msg, connect.CodeInvalidArgument)
}

func (i *ValidationInterceptor) validateResponse(msg any) error {
	if !i.validateResponses {
		return nil
	}
	return i.validate(msg, connect.CodeInternal)
}

func (i *ValidationInterceptor) validate(msg any, code connect.Code) error {
	if msg == nil {
		return nil
	}
	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return i.wrapValidationError(fmt.Errorf("expected proto.Message, got %T", msg), code)
	}
	// 1. Standard protovalidate rules
	if err := i.validator.Validate(protoMsg); err != nil {
		return i.wrapValidationError(err, code)
	}

	// 2. Deep Struct protection (the part protovalidate can't do yet)
	if err := validateAllStructs(protoMsg); err != nil {
		return i.wrapValidationError(err, code)
	}

	return nil
}

func (i *ValidationInterceptor) wrapValidationError(originalErr error, code connect.Code) error {
	connectErr := connect.NewError(code, originalErr)
	if i.noErrorDetails {
		return connectErr
	}

	var ve *protovalidate.ValidationError
	if errors.As(originalErr, &ve) {
		if detail, err := connect.NewErrorDetail(ve.ToProto()); err == nil {
			connectErr.AddDetail(detail)
		}
	}
	return connectErr
}

// validateAllStructs walks the message using only public reflection APIs.
func validateAllStructs(m proto.Message) error {
	return visitStructs(m.ProtoReflect())
}

//nolint:gocognit // this is a complex validation logic
func visitStructs(
	pr protoreflect.Message,
) error {
	// Check if the message itself is a Struct (message-level validation)
	if pr.Descriptor().FullName() == "google.protobuf.Struct" {
		s, ok := pr.Interface().(*structpb.Struct)
		if !ok {
			return fmt.Errorf("expected *structpb.Struct, got %T", pr.Interface())
		}
		if err := validateSingleStruct(s); err != nil {
			return err
		}
	}

	// Recurse into all fields (field-level validation)
	var visitErr error
	pr.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList():
			list := v.List()
			for i := 0; i < list.Len(); i++ { //nolint:intrange // Traditional for loop is appropriate for iterating over list length
				if fd.Message() != nil {
					if msg := list.Get(i).Message(); msg.IsValid() {
						if err := visitStructs(msg); err != nil {
							visitErr = err
							return false
						}
					}
				}
			}

		case fd.IsMap():
			mapVal := v.Map()
			mapVal.Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
				if fd.MapValue().Message() != nil {
					if msg := mv.Message(); msg.IsValid() {
						if err := visitStructs(msg); err != nil {
							visitErr = err
							return false
						}
					}
				}
				return true
			})
			if visitErr != nil {
				return false
			}

		case fd.Message() != nil:
			if msg := v.Message(); msg.IsValid() {
				if err := visitStructs(msg); err != nil {
					visitErr = err
					return false
				}
			}
		}
		return true
	})

	return visitErr
}

// validateSingleStruct applies all limits to one Struct.
func validateSingleStruct(s *structpb.Struct) error {
	// 1. Exact wire size ≤ 1 MiB
	if sz := proto.Size(s); sz > maxStructSizeB {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("google.protobuf.Struct exceeds 1 MiB (size: %d bytes)", sz))
	}

	// 2. Max 200 top-level keys
	if fields := s.GetFields(); len(fields) > maxStructFields {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("google.protobuf.Struct has too many top-level fields (%d > %d)", len(fields), maxStructFields))
	}

	return nil
}

type streamingClientInterceptor struct {
	connect.StreamingClientConn

	interceptor *ValidationInterceptor
}

func (s *streamingClientInterceptor) Send(msg any) error {
	if err := s.interceptor.validateRequest(msg); err != nil {
		return err
	}
	return s.StreamingClientConn.Send(msg)
}

func (s *streamingClientInterceptor) Receive(msg any) error {
	if err := s.StreamingClientConn.Receive(msg); err != nil {
		return err
	}
	return s.interceptor.validateResponse(msg)
}

type streamingHandlerInterceptor struct {
	connect.StreamingHandlerConn

	interceptor *ValidationInterceptor
}

func (s *streamingHandlerInterceptor) Send(msg any) error {
	if err := s.interceptor.validateResponse(msg); err != nil {
		return err
	}
	return s.StreamingHandlerConn.Send(msg)
}

func (s *streamingHandlerInterceptor) Receive(msg any) error {
	if err := s.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}
	return s.interceptor.validateRequest(msg)
}

type optionFunc func(*ValidationInterceptor)

func (f optionFunc) apply(i *ValidationInterceptor) { f(i) }
