package frameserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
)

// serverManager implements the ServerManager interface
type serverManager struct {
	config           Config
	logger           Logger
	metricsCollector MetricsCollector
	
	// Servers
	httpServer *http.Server
	grpcServer *grpc.Server
	grpcListener net.Listener
	
	// Server state
	running   bool
	startTime time.Time
	mutex     sync.RWMutex
	
	// Request tracking
	activeRequests *atomic.Int64
	totalRequests  *atomic.Int64
	errorCount     *atomic.Int64
}

// NewServerManager creates a new server manager instance
func NewServerManager(config Config, logger Logger, metricsCollector MetricsCollector) ServerManager {
	return &serverManager{
		config:           config,
		logger:           logger,
		metricsCollector: metricsCollector,
		activeRequests:   &atomic.Int64{},
		totalRequests:    &atomic.Int64{},
		errorCount:       &atomic.Int64{},
	}
}

// Start starts both HTTP and gRPC servers
func (sm *serverManager) Start(ctx context.Context) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.running {
		return nil // Already running
	}

	sm.startTime = time.Now()

	// Start HTTP server
	if err := sm.startHTTPServer(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start gRPC server
	if err := sm.startGRPCServer(ctx); err != nil {
		// Stop HTTP server if gRPC fails
		if sm.httpServer != nil {
			sm.httpServer.Shutdown(ctx)
		}
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	sm.running = true

	if sm.logger != nil {
		sm.logger.WithField("httpAddress", sm.GetHTTPAddress()).
			WithField("grpcAddress", sm.GetGRPCAddress()).
			Info("Server manager started successfully")
	}

	if sm.metricsCollector != nil {
		sm.metricsCollector.RecordServerStarted("http")
		sm.metricsCollector.RecordServerStarted("grpc")
	}

	return nil
}

// Stop stops both HTTP and gRPC servers
func (sm *serverManager) Stop(ctx context.Context) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if !sm.running {
		return nil // Already stopped
	}

	startTime := time.Now()
	var errors []error

	// Stop HTTP server
	if sm.httpServer != nil {
		if err := sm.httpServer.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop HTTP server: %w", err))
		}
		sm.httpServer = nil
	}

	// Stop gRPC server
	if sm.grpcServer != nil {
		sm.grpcServer.GracefulStop()
		sm.grpcServer = nil
	}

	// Close gRPC listener
	if sm.grpcListener != nil {
		if err := sm.grpcListener.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close gRPC listener: %w", err))
		}
		sm.grpcListener = nil
	}

	sm.running = false
	stopDuration := time.Since(startTime)

	if sm.logger != nil {
		sm.logger.WithField("duration", stopDuration).Info("Server manager stopped")
	}

	if sm.metricsCollector != nil {
		sm.metricsCollector.RecordServerStopped("http", stopDuration)
		sm.metricsCollector.RecordServerStopped("grpc", stopDuration)
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors occurred while stopping servers: %v", errors)
	}

	return nil
}

// IsRunning returns whether the server manager is running
func (sm *serverManager) IsRunning() bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.running
}

// GetHTTPAddress returns the HTTP server address
func (sm *serverManager) GetHTTPAddress() string {
	if sm.config == nil {
		return ""
	}
	
	address := sm.config.GetHTTPAddress()
	if address != "" {
		return address
	}
	
	port := sm.config.GetHTTPPort()
	if port > 0 {
		return fmt.Sprintf(":%d", port)
	}
	
	return ":8080" // Default HTTP port
}

// GetGRPCAddress returns the gRPC server address
func (sm *serverManager) GetGRPCAddress() string {
	if sm.config == nil {
		return ""
	}
	
	address := sm.config.GetGRPCAddress()
	if address != "" {
		return address
	}
	
	port := sm.config.GetGRPCPort()
	if port > 0 {
		return fmt.Sprintf(":%d", port)
	}
	
	return ":9090" // Default gRPC port
}

// GetHTTPServer returns the HTTP server instance
func (sm *serverManager) GetHTTPServer() *http.Server {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.httpServer
}

// GetGRPCServer returns the gRPC server instance
func (sm *serverManager) GetGRPCServer() *grpc.Server {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.grpcServer
}

// IsHealthy returns the health status of the servers
func (sm *serverManager) IsHealthy(ctx context.Context) bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	if !sm.running {
		return false
	}
	
	// Check if servers are responsive
	// This is a basic implementation - in practice you might want more sophisticated health checks
	return sm.httpServer != nil && sm.grpcServer != nil
}

// GetServerStats returns server statistics
func (sm *serverManager) GetServerStats() ServerStats {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	var uptime time.Duration
	if sm.running && !sm.startTime.IsZero() {
		uptime = time.Since(sm.startTime)
	}
	
	return ServerStats{
		HTTPAddress:    sm.GetHTTPAddress(),
		GRPCAddress:    sm.GetGRPCAddress(),
		StartTime:      sm.startTime,
		Uptime:         uptime,
		IsRunning:      sm.running,
		ActiveRequests: sm.activeRequests.Load(),
		TotalRequests:  sm.totalRequests.Load(),
		ErrorCount:     sm.errorCount.Load(),
	}
}

// startHTTPServer starts the HTTP server
func (sm *serverManager) startHTTPServer(ctx context.Context) error {
	builder := NewHTTPServerBuilder(sm.config, sm.logger)
	
	// Add request tracking middleware
	builder = builder.WithMiddleware(sm.requestTrackingMiddleware())
	
	// Add metrics middleware if available
	if sm.metricsCollector != nil {
		builder = builder.WithMiddleware(sm.metricsMiddleware())
	}
	
	// Configure timeouts
	if sm.config != nil {
		readTimeout := sm.config.GetHTTPReadTimeout()
		writeTimeout := sm.config.GetHTTPWriteTimeout()
		idleTimeout := sm.config.GetHTTPIdleTimeout()
		
		if readTimeout > 0 || writeTimeout > 0 || idleTimeout > 0 {
			builder = builder.WithTimeout(readTimeout, writeTimeout, idleTimeout)
		}
	}
	
	// Configure TLS if enabled
	if sm.config != nil && sm.config.IsRunSecurely() {
		certFile := sm.config.GetTLSCertFile()
		keyFile := sm.config.GetTLSKeyFile()
		
		if certFile != "" && keyFile != "" {
			builder = builder.WithTLS(certFile, keyFile)
		}
	}
	
	// Set address
	builder = builder.WithAddress(sm.GetHTTPAddress())
	
	// Build server
	server, err := builder.Build()
	if err != nil {
		return err
	}
	
	sm.httpServer = server
	
	// Start server in background
	go func() {
		var err error
		if sm.config != nil && sm.config.IsRunSecurely() {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		
		if err != nil && err != http.ErrServerClosed {
			if sm.logger != nil {
				sm.logger.WithError(err).Error("HTTP server error")
			}
		}
	}()
	
	if sm.logger != nil {
		sm.logger.WithField("address", sm.GetHTTPAddress()).Info("HTTP server started")
	}
	
	return nil
}

// startGRPCServer starts the gRPC server
func (sm *serverManager) startGRPCServer(ctx context.Context) error {
	builder := NewGRPCServerBuilder(sm.config, sm.logger)
	
	// Add metrics interceptors if available
	if sm.metricsCollector != nil {
		builder = builder.WithUnaryInterceptors(sm.grpcUnaryMetricsInterceptor())
		builder = builder.WithStreamInterceptors(sm.grpcStreamMetricsInterceptor())
	}
	
	// Configure TLS if enabled
	if sm.config != nil && sm.config.IsRunSecurely() {
		certFile := sm.config.GetTLSCertFile()
		keyFile := sm.config.GetTLSKeyFile()
		
		if certFile != "" && keyFile != "" {
			builder = builder.WithTLS(certFile, keyFile)
		}
	}
	
	// Set address
	builder = builder.WithAddress(sm.GetGRPCAddress())
	
	// Build server
	server, listener, err := builder.Build()
	if err != nil {
		return err
	}
	
	sm.grpcServer = server
	sm.grpcListener = listener
	
	// Start server in background
	go func() {
		if err := server.Serve(listener); err != nil {
			if sm.logger != nil {
				sm.logger.WithError(err).Error("gRPC server error")
			}
		}
	}()
	
	if sm.logger != nil {
		sm.logger.WithField("address", sm.GetGRPCAddress()).Info("gRPC server started")
	}
	
	return nil
}

// requestTrackingMiddleware creates middleware for tracking HTTP requests
func (sm *serverManager) requestTrackingMiddleware() MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sm.activeRequests.Add(1)
			sm.totalRequests.Add(1)
			
			defer sm.activeRequests.Add(-1)
			
			// Update metrics collector if available
			if sm.metricsCollector != nil {
				sm.metricsCollector.RecordActiveConnections(sm.activeRequests.Load())
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// metricsMiddleware creates middleware for collecting HTTP metrics
func (sm *serverManager) metricsMiddleware() MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Wrap response writer to capture status code
			wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			
			next.ServeHTTP(wrapper, r)
			
			duration := time.Since(start)
			
			// Record metrics
			sm.metricsCollector.RecordHTTPRequest(r.Method, r.URL.Path, wrapper.statusCode, duration)
			
			// Record error if status code indicates error
			if wrapper.statusCode >= 400 {
				sm.errorCount.Add(1)
			}
		})
	}
}

// grpcUnaryMetricsInterceptor creates a unary interceptor for gRPC metrics
func (sm *serverManager) grpcUnaryMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		
		resp, err := handler(ctx, req)
		
		duration := time.Since(start)
		
		if err != nil {
			sm.metricsCollector.RecordGRPCError(info.FullMethod, err)
			sm.errorCount.Add(1)
		} else {
			sm.metricsCollector.RecordGRPCRequest(info.FullMethod, duration)
		}
		
		return resp, err
	}
}

// grpcStreamMetricsInterceptor creates a stream interceptor for gRPC metrics
func (sm *serverManager) grpcStreamMetricsInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		
		err := handler(srv, ss)
		
		duration := time.Since(start)
		
		if err != nil {
			sm.metricsCollector.RecordGRPCError(info.FullMethod, err)
			sm.errorCount.Add(1)
		} else {
			sm.metricsCollector.RecordGRPCRequest(info.FullMethod, duration)
		}
		
		return err
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
