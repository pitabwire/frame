package frameserver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

// httpServerBuilder implements the HTTPServerBuilder interface
type httpServerBuilder struct {
	config     Config
	logger     Logger
	
	handler    http.Handler
	middleware []MiddlewareFunc
	address    string
	
	// Timeout configuration
	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration
	
	// TLS configuration
	certFile string
	keyFile  string
}

// NewHTTPServerBuilder creates a new HTTP server builder
func NewHTTPServerBuilder(config Config, logger Logger) HTTPServerBuilder {
	return &httpServerBuilder{
		config: config,
		logger: logger,
	}
}

// WithHandler sets the HTTP handler
func (b *httpServerBuilder) WithHandler(handler http.Handler) HTTPServerBuilder {
	b.handler = handler
	return b
}

// WithMiddleware adds middleware to the server
func (b *httpServerBuilder) WithMiddleware(middleware ...MiddlewareFunc) HTTPServerBuilder {
	b.middleware = append(b.middleware, middleware...)
	return b
}

// WithTimeout sets the server timeouts
func (b *httpServerBuilder) WithTimeout(read, write, idle time.Duration) HTTPServerBuilder {
	b.readTimeout = read
	b.writeTimeout = write
	b.idleTimeout = idle
	return b
}

// WithTLS configures TLS for the server
func (b *httpServerBuilder) WithTLS(certFile, keyFile string) HTTPServerBuilder {
	b.certFile = certFile
	b.keyFile = keyFile
	return b
}

// WithAddress sets the server address
func (b *httpServerBuilder) WithAddress(address string) HTTPServerBuilder {
	b.address = address
	return b
}

// Build creates the HTTP server
func (b *httpServerBuilder) Build() (*http.Server, error) {
	// Create default handler if none provided
	handler := b.handler
	if handler == nil {
		handler = http.DefaultServeMux
		
		if b.logger != nil {
			b.logger.Warn("No HTTP handler provided, using default mux")
		}
	}
	
	// Apply middleware in reverse order (last added is outermost)
	for i := len(b.middleware) - 1; i >= 0; i-- {
		handler = b.middleware[i](handler)
	}
	
	// Create server
	server := &http.Server{
		Addr:    b.address,
		Handler: handler,
	}
	
	// Configure timeouts
	if b.readTimeout > 0 {
		server.ReadTimeout = b.readTimeout
	}
	if b.writeTimeout > 0 {
		server.WriteTimeout = b.writeTimeout
	}
	if b.idleTimeout > 0 {
		server.IdleTimeout = b.idleTimeout
	}
	
	// Configure TLS if provided
	if b.certFile != "" && b.keyFile != "" {
		// Load TLS configuration
		cert, err := tls.LoadX509KeyPair(b.certFile, b.keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificates: %w", err)
		}
		
		server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12, // Enforce minimum TLS version
		}
		
		if b.logger != nil {
			b.logger.WithField("certFile", b.certFile).WithField("keyFile", b.keyFile).Info("TLS configured for HTTP server")
		}
	}
	
	if b.logger != nil {
		b.logger.WithField("address", b.address).WithField("tlsEnabled", b.certFile != "").Debug("HTTP server built successfully")
	}
	
	return server, nil
}

// DefaultHTTPMiddleware provides common HTTP middleware
func DefaultHTTPMiddleware(logger Logger) []MiddlewareFunc {
	var middleware []MiddlewareFunc
	
	// Request logging middleware
	if logger != nil {
		middleware = append(middleware, LoggingMiddleware(logger))
	}
	
	// Recovery middleware (should be first/outermost)
	middleware = append(middleware, RecoveryMiddleware(logger))
	
	return middleware
}

// LoggingMiddleware creates middleware for request logging
func LoggingMiddleware(logger Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Wrap response writer to capture status code
			wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			
			next.ServeHTTP(wrapper, r)
			
			duration := time.Since(start)
			
			logger.WithField("method", r.Method).
				WithField("path", r.URL.Path).
				WithField("status", wrapper.statusCode).
				WithField("duration", duration).
				WithField("remoteAddr", r.RemoteAddr).
				WithField("userAgent", r.UserAgent()).
				Info("HTTP request processed")
		})
	}
}

// RecoveryMiddleware creates middleware for panic recovery
func RecoveryMiddleware(logger Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					if logger != nil {
						logger.WithField("method", r.Method).
							WithField("path", r.URL.Path).
							WithField("panic", err).
							Error("Panic recovered in HTTP handler")
					}
					
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal Server Error"))
				}
			}()
			
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware creates CORS middleware
func CORSMiddleware(corsManager CORSManager) MiddlewareFunc {
	if corsManager == nil || !corsManager.IsEnabled() {
		// Return no-op middleware if CORS is disabled
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	
	return corsManager.Middleware()
}

// AuthenticationMiddleware creates authentication middleware
func AuthenticationMiddleware(authenticator Authenticator) MiddlewareFunc {
	if authenticator == nil {
		// Return no-op middleware if authenticator is not provided
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	
	return authenticator.HTTPMiddleware()
}

// AuthorizationMiddleware creates authorization middleware
func AuthorizationMiddleware(authorizer Authorizer, action, resource string) MiddlewareFunc {
	if authorizer == nil {
		// Return no-op middleware if authorizer is not provided
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	
	return authorizer.HTTPMiddleware(action, resource)
}
