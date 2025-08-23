package frameserver

import (
	"context"

	"github.com/pitabwire/frame/internal/common"
)

// ServiceRegistry defines the interface for service registration needed by server functionality
type ServiceRegistry interface {
	// RegisterServerManager registers the server manager with the service
	RegisterServerManager(serverManager ServerManager)
	
	// GetConfig returns the service configuration
	GetConfig() Config
	
	// GetLogger returns the service logger
	GetLogger() Logger
	
	// GetMetricsCollector returns the metrics collector if available
	GetMetricsCollector() MetricsCollector
	
	// GetAuthenticator returns the authenticator if available
	GetAuthenticator() Authenticator
	
	// GetAuthorizer returns the authorizer if available
	GetAuthorizer() Authorizer
	
	// GetHealthChecker returns the health checker if available
	GetHealthChecker() HealthChecker
	
	// GetCORSManager returns the CORS manager if available
	GetCORSManager() CORSManager
	
	// GetServiceRegistrar returns the gRPC service registrar if available
	GetServiceRegistrar() ServiceRegistrar
}

// WithHTTPServer returns an option function that enables HTTP server functionality
func WithHTTPServer() func(ctx context.Context, service ServiceRegistry) error {
	return func(ctx context.Context, service ServiceRegistry) error {
		config := service.GetConfig()
		logger := service.GetLogger()
		metricsCollector := service.GetMetricsCollector()
		
		// Create server manager
		commonServerManager := common.NewServerManager(config, logger, metricsCollector)
		
		// Create adapter to bridge common.ServerManager to local ServerManager interface
		serverManager := &serverManagerAdapter{commonServerManager}
		
		// Register with service
		service.RegisterServerManager(serverManager)
		
		if logger != nil {
			logger.Info("HTTP server functionality enabled successfully")
		}
		
		return nil
	}
}

// WithGRPCServer returns an option function that enables gRPC server functionality
func WithGRPCServer() func(ctx context.Context, service ServiceRegistry) error {
	return func(ctx context.Context, service ServiceRegistry) error {
		config := service.GetConfig()
		logger := service.GetLogger()
		metricsCollector := service.GetMetricsCollector()
		
		// Create server manager
		commonServerManager := common.NewServerManager(config, logger, metricsCollector)
		
		// Create adapter to bridge common.ServerManager to local ServerManager interface
		serverManager := &serverManagerAdapter{commonServerManager}
		
		// Register with service
		service.RegisterServerManager(serverManager)
		
		if logger != nil {
			logger.Info("gRPC server functionality enabled successfully")
		}
		
		return nil
	}
}

// WithServer returns an option function that enables both HTTP and gRPC server functionality
func WithServer() func(ctx context.Context, service ServiceRegistry) error {
	return func(ctx context.Context, service ServiceRegistry) error {
		config := service.GetConfig()
		logger := service.GetLogger()
		metricsCollector := service.GetMetricsCollector()
		
		// Create server manager
		commonServerManager := common.NewServerManager(config, logger, metricsCollector)
		
		// Start the server manager
		if err := commonServerManager.Start(ctx); err != nil {
			if logger != nil {
				logger.WithError(err).Error("Failed to start server manager")
			}
			return err
		}
		
		// Create adapter to bridge common.ServerManager to local ServerManager interface
		serverManager := &serverManagerAdapter{commonServerManager}
		
		// Register with service
		service.RegisterServerManager(serverManager)
		
		if logger != nil {
			logger.Info("Server functionality enabled and started successfully")
		}
		
		return nil
	}
}

// WithServerAutoStart returns an option function that enables and automatically starts server functionality
func WithServerAutoStart() func(ctx context.Context, service ServiceRegistry) error {
	return WithServer() // Same as WithServer for now, but could be extended with additional auto-start logic
}

// serverManagerAdapter adapts common.ServerManager to local ServerManager interface
type serverManagerAdapter struct {
	common.ServerManager
}

// GetServerStats adapts common.ServerStats to local ServerStats
func (s *serverManagerAdapter) GetServerStats() ServerStats {
	commonStats := s.ServerManager.GetServerStats()
	return ServerStats{
		HTTPAddress:    s.GetHTTPAddress(),
		GRPCAddress:    s.GetGRPCAddress(),
		IsRunning:      true, // TODO: implement proper status tracking
		ActiveRequests: int64(commonStats.GetConnectionCount()),
		TotalRequests:  commonStats.GetRequestCount(),
	}
}

// IsRunning implements local ServerManager interface
func (s *serverManagerAdapter) IsRunning() bool {
	return true // TODO: implement proper status tracking
}

// IsHealthy implements local ServerManager interface
func (s *serverManagerAdapter) IsHealthy(ctx context.Context) bool {
	return true // TODO: implement proper health checking
}
