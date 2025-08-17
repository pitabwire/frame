package frameserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// grpcServerBuilder implements the GRPCServerBuilder interface
type grpcServerBuilder struct {
	config  Config
	logger  Logger
	
	address             string
	unaryInterceptors   []grpc.UnaryServerInterceptor
	streamInterceptors  []grpc.StreamServerInterceptor
	options             []grpc.ServerOption
	serviceRegistrar    ServiceRegistrar
	
	// TLS configuration
	certFile string
	keyFile  string
}

// NewGRPCServerBuilder creates a new gRPC server builder
func NewGRPCServerBuilder(config Config, logger Logger) GRPCServerBuilder {
	return &grpcServerBuilder{
		config: config,
		logger: logger,
	}
}

// WithUnaryInterceptors adds unary interceptors to the server
func (b *grpcServerBuilder) WithUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) GRPCServerBuilder {
	b.unaryInterceptors = append(b.unaryInterceptors, interceptors...)
	return b
}

// WithStreamInterceptors adds stream interceptors to the server
func (b *grpcServerBuilder) WithStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) GRPCServerBuilder {
	b.streamInterceptors = append(b.streamInterceptors, interceptors...)
	return b
}

// WithTLS configures TLS for the server
func (b *grpcServerBuilder) WithTLS(certFile, keyFile string) GRPCServerBuilder {
	b.certFile = certFile
	b.keyFile = keyFile
	return b
}

// WithAddress sets the server address
func (b *grpcServerBuilder) WithAddress(address string) GRPCServerBuilder {
	b.address = address
	return b
}

// WithOptions adds gRPC server options
func (b *grpcServerBuilder) WithOptions(options ...grpc.ServerOption) GRPCServerBuilder {
	b.options = append(b.options, options...)
	return b
}

// WithServiceRegistrar sets the service registrar
func (b *grpcServerBuilder) WithServiceRegistrar(registrar ServiceRegistrar) GRPCServerBuilder {
	b.serviceRegistrar = registrar
	return b
}

// Build creates the gRPC server and listener
func (b *grpcServerBuilder) Build() (*grpc.Server, net.Listener, error) {
	// Create listener
	listener, err := net.Listen("tcp", b.address)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create listener on %s: %w", b.address, err)
	}
	
	// Prepare server options
	var opts []grpc.ServerOption
	
	// Add provided options
	opts = append(opts, b.options...)
	
	// Configure TLS if provided
	if b.certFile != "" && b.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(b.certFile, b.keyFile)
		if err != nil {
			listener.Close()
			return nil, nil, fmt.Errorf("failed to load TLS certificates: %w", err)
		}
		
		creds := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
		
		opts = append(opts, grpc.Creds(creds))
		
		if b.logger != nil {
			b.logger.WithField("certFile", b.certFile).WithField("keyFile", b.keyFile).Info("TLS configured for gRPC server")
		}
	}
	
	// Add interceptors
	if len(b.unaryInterceptors) > 0 {
		if len(b.unaryInterceptors) == 1 {
			opts = append(opts, grpc.UnaryInterceptor(b.unaryInterceptors[0]))
		} else {
			opts = append(opts, grpc.ChainUnaryInterceptor(b.unaryInterceptors...))
		}
	}
	
	if len(b.streamInterceptors) > 0 {
		if len(b.streamInterceptors) == 1 {
			opts = append(opts, grpc.StreamInterceptor(b.streamInterceptors[0]))
		} else {
			opts = append(opts, grpc.ChainStreamInterceptor(b.streamInterceptors...))
		}
	}
	
	// Create server
	server := grpc.NewServer(opts...)
	
	// Register services if registrar is provided
	if b.serviceRegistrar != nil {
		b.serviceRegistrar.RegisterServices(server)
		
		if b.logger != nil {
			b.logger.Info("gRPC services registered")
		}
	}
	
	if b.logger != nil {
		b.logger.WithField("address", b.address).WithField("tlsEnabled", b.certFile != "").Debug("gRPC server built successfully")
	}
	
	return server, listener, nil
}

// DefaultGRPCInterceptors provides common gRPC interceptors
func DefaultGRPCInterceptors(logger Logger) ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	var unaryInterceptors []grpc.UnaryServerInterceptor
	var streamInterceptors []grpc.StreamServerInterceptor
	
	// Logging interceptors
	if logger != nil {
		unaryInterceptors = append(unaryInterceptors, LoggingUnaryInterceptor(logger))
		streamInterceptors = append(streamInterceptors, LoggingStreamInterceptor(logger))
	}
	
	// Recovery interceptors (should be first/outermost)
	unaryInterceptors = append(unaryInterceptors, RecoveryUnaryInterceptor(logger))
	streamInterceptors = append(streamInterceptors, RecoveryStreamInterceptor(logger))
	
	return unaryInterceptors, streamInterceptors
}

// LoggingUnaryInterceptor creates a unary interceptor for request logging
func LoggingUnaryInterceptor(logger Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		
		resp, err := handler(ctx, req)
		
		duration := time.Since(start)
		
		logEntry := logger.WithField("method", info.FullMethod).
			WithField("duration", duration)
		
		if err != nil {
			logEntry.WithError(err).Error("gRPC unary request failed")
		} else {
			logEntry.Info("gRPC unary request processed")
		}
		
		return resp, err
	}
}

// LoggingStreamInterceptor creates a stream interceptor for request logging
func LoggingStreamInterceptor(logger Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		
		err := handler(srv, ss)
		
		duration := time.Since(start)
		
		logEntry := logger.WithField("method", info.FullMethod).
			WithField("duration", duration)
		
		if err != nil {
			logEntry.WithError(err).Error("gRPC stream request failed")
		} else {
			logEntry.Info("gRPC stream request processed")
		}
		
		return err
	}
}

// RecoveryUnaryInterceptor creates a unary interceptor for panic recovery
func RecoveryUnaryInterceptor(logger Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.WithField("method", info.FullMethod).
						WithField("panic", r).
						Error("Panic recovered in gRPC unary handler")
				}
				
				err = fmt.Errorf("internal server error")
			}
		}()
		
		return handler(ctx, req)
	}
}

// RecoveryStreamInterceptor creates a stream interceptor for panic recovery
func RecoveryStreamInterceptor(logger Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.WithField("method", info.FullMethod).
						WithField("panic", r).
						Error("Panic recovered in gRPC stream handler")
				}
				
				err = fmt.Errorf("internal server error")
			}
		}()
		
		return handler(srv, ss)
	}
}

// AuthenticationUnaryInterceptor creates authentication interceptor for unary calls
func AuthenticationUnaryInterceptor(authenticator Authenticator) grpc.UnaryServerInterceptor {
	if authenticator == nil {
		// Return no-op interceptor if authenticator is not provided
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}
	
	return authenticator.UnaryInterceptor()
}

// AuthenticationStreamInterceptor creates authentication interceptor for stream calls
func AuthenticationStreamInterceptor(authenticator Authenticator) grpc.StreamServerInterceptor {
	if authenticator == nil {
		// Return no-op interceptor if authenticator is not provided
		return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	}
	
	return authenticator.StreamInterceptor()
}
