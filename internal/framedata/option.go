package framedata

import (
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

// WithDatastore function moved to datastore.go to avoid duplication
