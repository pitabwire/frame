package framedata

import (
	"context"
	"io/fs"
)

// ServiceRegistry defines the interface for service registration needed by datastore functionality
type ServiceRegistry interface {
	// RegisterDatastoreManager registers the datastore manager with the service
	RegisterDatastoreManager(datastoreManager DatastoreManager)
	
	// RegisterMigrator registers the migrator with the service
	RegisterMigrator(migrator Migrator)
	
	// GetConfig returns the service configuration
	GetConfig() Config
	
	// GetLogger returns the service logger
	GetLogger() Logger
	
	// GetMetricsCollector returns the metrics collector if available
	GetMetricsCollector() MetricsCollector
	
	// GetMigrationsFS returns the migrations filesystem if available
	GetMigrationsFS() fs.FS
}

// WithDatastore returns an option function that enables datastore functionality
func WithDatastore() func(ctx context.Context, service ServiceRegistry) error {
	return func(ctx context.Context, service ServiceRegistry) error {
		config := service.GetConfig()
		logger := service.GetLogger()
		metricsCollector := service.GetMetricsCollector()
		
		// Create datastore manager
		datastoreManager := NewDatastoreManager(config, logger, metricsCollector)
		
		// Initialize datastore
		if err := datastoreManager.Initialize(ctx); err != nil {
			if logger != nil {
				logger.WithError(err).Error("Failed to initialize datastore")
			}
			return err
		}
		
		// Register with service
		service.RegisterDatastoreManager(datastoreManager)
		
		// Set up migrator if migrations filesystem is available
		migrationsFS := service.GetMigrationsFS()
		if migrationsFS != nil {
			migrator := NewMigrator(datastoreManager, config, logger, migrationsFS)
			
			// Register migrator with service
			service.RegisterMigrator(migrator)
			
			if logger != nil {
				logger.Info("Database migrator enabled")
			}
		}
		
		if logger != nil {
			logger.Info("Datastore functionality enabled successfully")
		}
		
		return nil
	}
}

// WithMigrations returns an option function that runs database migrations
func WithMigrations() func(ctx context.Context, service ServiceRegistry) error {
	return func(ctx context.Context, service ServiceRegistry) error {
		logger := service.GetLogger()
		
		// This option assumes datastore has already been configured
		// In a real implementation, you would get the migrator from the service registry
		// For now, we'll create a new one if needed
		
		config := service.GetConfig()
		migrationsFS := service.GetMigrationsFS()
		
		if migrationsFS == nil {
			if logger != nil {
				logger.Warn("No migrations filesystem configured, skipping migrations")
			}
			return nil
		}
		
		// We need a datastore manager to create the migrator
		// In practice, this would be retrieved from the service registry
		datastoreManager := NewDatastoreManager(config, logger, service.GetMetricsCollector())
		if err := datastoreManager.Initialize(ctx); err != nil {
			return err
		}
		
		migrator := NewMigrator(datastoreManager, config, logger, migrationsFS)
		
		// Run migrations
		if err := migrator.Migrate(ctx); err != nil {
			if logger != nil {
				logger.WithError(err).Error("Failed to run database migrations")
			}
			return err
		}
		
		if logger != nil {
			logger.Info("Database migrations completed successfully")
		}
		
		return nil
	}
}
