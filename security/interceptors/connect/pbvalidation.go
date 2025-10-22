package connect

import (
	"context"
	"fmt"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

// ValidationOptions configures the validation interceptor.
type ValidationOptions struct {
	// ValidateRequests enables request validation (default: true)
	ValidateRequests bool
	// ValidateResponses enables response validation (default: true)
	ValidateResponses bool
	// FailOnValidationError determines if validation errors should fail the request (default: true)
	FailOnValidationError bool
	// Logger for validation errors (optional)
	Logger ValidationLogger
}

// ValidationLogger is an optional interface for logging validation errors.
type ValidationLogger interface {
	LogValidationError(ctx context.Context, direction string, err error)
}

// DefaultValidationOptions returns the default validation options.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		ValidateRequests:      true,
		ValidateResponses:     true,
		FailOnValidationError: true,
	}
}

// LanguageInterceptor implements connect.Interceptor for protovalidate validation.
type ValidationInterceptor struct {
	validator protovalidate.Validator
	opts      ValidationOptions
}

// NewValidationInterceptor creates a new validation interceptor with default options.
func NewValidationInterceptor() (*ValidationInterceptor, error) {
	return NewValidationInterceptorWithOptions(DefaultValidationOptions())
}

// NewValidationInterceptorWithOptions creates a new validation interceptor with custom options.
func NewValidationInterceptorWithOptions(opts ValidationOptions) (*ValidationInterceptor, error) {
	validator, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create protovalidate validator: %w", err)
	}

	return &ValidationInterceptor{
		validator: validator,
		opts:      opts,
	}, nil
}

// validateMessage validates a proto message and returns an appropriate error.
func (v *ValidationInterceptor) validateMessage(
	ctx context.Context,
	msg any,
	direction string,
	errorCode connect.Code,
) error {
	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return nil // Not a proto message, skip validation
	}

	if err := v.validator.Validate(protoMsg); err != nil {
		// Log if logger is available
		if v.opts.Logger != nil {
			v.opts.Logger.LogValidationError(ctx, direction, err)
		}

		if v.opts.FailOnValidationError {
			return connect.NewError(
				errorCode,
				fmt.Errorf("%s validation failed: %w", direction, err),
			)
		}
	}

	return nil
}

// WrapUnary validates unary requests and responses.
func (v *ValidationInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// Validate request
		if v.opts.ValidateRequests {
			if err := v.validateMessage(ctx, req.Any(), "request", connect.CodeInvalidArgument); err != nil {
				return nil, err
			}
		}

		// Call the handler
		resp, err := next(ctx, req)
		if err != nil {
			return nil, err
		}

		// Validate response
		if v.opts.ValidateResponses {
			if err = v.validateMessage(ctx, resp.Any(), "response", connect.CodeInternal); err != nil {
				return nil, err
			}
		}

		return resp, nil
	}
}

// WrapStreamingClient validates streaming client messages (pass-through for server-side).
func (v *ValidationInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler validates streaming messages.
func (v *ValidationInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		// Wrap the connection to intercept Receive and Send calls
		wrappedConn := &validatingStreamConn{
			StreamingHandlerConn: conn,
			interceptor:          v,
			ctx:                  ctx,
		}

		return next(ctx, wrappedConn)
	}
}

// languageStreamConn wraps a StreamingHandlerConn to validate messages.
type validatingStreamConn struct {
	connect.StreamingHandlerConn
	interceptor *ValidationInterceptor
	ctx         context.Context
}

// Receive validates incoming stream messages.
func (v *validatingStreamConn) Receive(msg any) error {
	// Validate the received message
	if v.interceptor.opts.ValidateRequests {
		if err := v.interceptor.validateMessage(
			v.ctx,
			msg,
			"stream request",
			connect.CodeInvalidArgument,
		); err != nil {
			return err
		}
	}

	if err := v.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}

	return nil
}

// Send validates outgoing stream messages.
func (v *validatingStreamConn) Send(msg any) error {
	// Validate before sending
	if v.interceptor.opts.ValidateResponses {
		if err := v.interceptor.validateMessage(
			v.ctx,
			msg,
			"stream response",
			connect.CodeInternal,
		); err != nil {
			return err
		}
	}

	return v.StreamingHandlerConn.Send(msg)
}
