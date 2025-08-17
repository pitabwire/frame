package framedata

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// datastoreManager implements the DatastoreManager interface
type datastoreManager struct {
	config           Config
	logger           Logger
	metricsCollector MetricsCollector
	
	// Database connections
	writeDB   *sql.DB
	readDB    *sql.DB
	
	// Connection state
	initialized bool
	mutex       sync.RWMutex
	
	// Health monitoring
	healthChecker HealthChecker
}

// NewDatastoreManager creates a new datastore manager instance
func NewDatastoreManager(config Config, logger Logger, metricsCollector MetricsCollector) DatastoreManager {
	return &datastoreManager{
		config:           config,
		logger:           logger,
		metricsCollector: metricsCollector,
	}
}

// Initialize initializes the datastore connections
func (dm *datastoreManager) Initialize(ctx context.Context) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if dm.initialized {
		return nil
	}

	// Initialize write database connection
	if err := dm.initializeWriteConnection(ctx); err != nil {
		return fmt.Errorf("failed to initialize write connection: %w", err)
	}

	// Initialize read database connection (optional)
	if err := dm.initializeReadConnection(ctx); err != nil {
		if dm.logger != nil {
			dm.logger.WithError(err).Warn("Failed to initialize read-only connection, using write connection for reads")
		}
		dm.readDB = dm.writeDB
	}

	// Start health monitoring if configured
	if dm.config.GetDatabaseHealthCheckInterval() > 0 {
		healthChecker := NewHealthChecker(dm, dm.logger)
		dm.healthChecker = healthChecker
		
		if err := healthChecker.StartHealthMonitoring(ctx, dm.config.GetDatabaseHealthCheckInterval()); err != nil {
			if dm.logger != nil {
				dm.logger.WithError(err).Warn("Failed to start health monitoring")
			}
		}
	}

	dm.initialized = true

	if dm.logger != nil {
		dm.logger.Info("Datastore manager initialized successfully")
	}

	return nil
}

// initializeWriteConnection initializes the write database connection
func (dm *datastoreManager) initializeWriteConnection(ctx context.Context) error {
	if dm.config == nil {
		return fmt.Errorf("datastore config is not configured")
	}
	
	databaseURL := dm.config.GetDatabaseURL()
	if databaseURL == "" {
		return fmt.Errorf("database URL is not configured")
	}

	driver := dm.config.GetDatabaseDriver()
	if driver == "" {
		driver = "postgres" // Default to PostgreSQL
	}

	db, err := sql.Open(driver, databaseURL)
	if err != nil {
		if dm.logger != nil {
			dm.logger.WithError(err).WithField("driver", driver).Error("Failed to open write database connection")
		}
		return err
	}

	// Configure connection pool
	dm.configureConnectionPool(db)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		if dm.logger != nil {
			dm.logger.WithError(err).Error("Failed to ping write database")
		}
		return err
	}

	dm.writeDB = db

	if dm.logger != nil {
		dm.logger.WithField("driver", driver).Info("Write database connection initialized")
	}

	return nil
}

// initializeReadConnection initializes the read-only database connection
func (dm *datastoreManager) initializeReadConnection(ctx context.Context) error {
	readOnlyURL := dm.config.GetDatabaseReadOnlyURL()
	if readOnlyURL == "" {
		return nil // No read-only connection configured
	}

	driver := dm.config.GetDatabaseDriver()
	if driver == "" {
		driver = "postgres"
	}

	db, err := sql.Open(driver, readOnlyURL)
	if err != nil {
		return err
	}

	// Configure connection pool
	dm.configureConnectionPool(db)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return err
	}

	dm.readDB = db

	if dm.logger != nil {
		dm.logger.WithField("driver", driver).Info("Read-only database connection initialized")
	}

	return nil
}

// configureConnectionPool configures the database connection pool
func (dm *datastoreManager) configureConnectionPool(db *sql.DB) {
	if maxOpen := dm.config.GetDatabaseMaxOpenConnections(); maxOpen > 0 {
		db.SetMaxOpenConns(maxOpen)
	}

	if maxIdle := dm.config.GetDatabaseMaxIdleConnections(); maxIdle > 0 {
		db.SetMaxIdleConns(maxIdle)
	}

	if maxLifetime := dm.config.GetDatabaseConnectionMaxLifetime(); maxLifetime > 0 {
		db.SetConnMaxLifetime(maxLifetime)
	}

	if maxIdleTime := dm.config.GetDatabaseConnectionMaxIdleTime(); maxIdleTime > 0 {
		db.SetConnMaxIdleTime(maxIdleTime)
	}
}

// GetConnection returns a write database connection
func (dm *datastoreManager) GetConnection(ctx context.Context) (*sql.DB, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	if !dm.initialized {
		return nil, fmt.Errorf("datastore manager is not initialized")
	}

	if dm.writeDB == nil {
		return nil, fmt.Errorf("write database connection is not available")
	}

	// Record metrics if available
	if dm.metricsCollector != nil {
		start := time.Now()
		defer func() {
			dm.metricsCollector.RecordConnectionAcquired(time.Since(start))
		}()
	}

	return dm.writeDB, nil
}

// GetReadOnlyConnection returns a read-only database connection
func (dm *datastoreManager) GetReadOnlyConnection(ctx context.Context) (*sql.DB, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	if !dm.initialized {
		return nil, fmt.Errorf("datastore manager is not initialized")
	}

	if dm.readDB == nil {
		return nil, fmt.Errorf("read database connection is not available")
	}

	// Record metrics if available
	if dm.metricsCollector != nil {
		start := time.Now()
		defer func() {
			dm.metricsCollector.RecordConnectionAcquired(time.Since(start))
		}()
	}

	return dm.readDB, nil
}

// IsHealthy returns the health status of the datastore
func (dm *datastoreManager) IsHealthy(ctx context.Context) bool {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	if !dm.initialized {
		return false
	}

	if dm.healthChecker != nil {
		return dm.healthChecker.IsHealthy()
	}

	// Fallback health check
	if dm.writeDB != nil {
		if err := dm.writeDB.PingContext(ctx); err != nil {
			return false
		}
	}

	if dm.readDB != nil && dm.readDB != dm.writeDB {
		if err := dm.readDB.PingContext(ctx); err != nil {
			return false
		}
	}

	return true
}

// BeginTransaction starts a new write transaction
func (dm *datastoreManager) BeginTransaction(ctx context.Context) (*sql.Tx, error) {
	db, err := dm.GetConnection(ctx)
	if err != nil {
		return nil, err
	}

	if dm.metricsCollector != nil {
		dm.metricsCollector.RecordTransactionStarted()
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		if dm.metricsCollector != nil {
			dm.metricsCollector.RecordConnectionError(err)
		}
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return tx, nil
}

// BeginReadOnlyTransaction starts a new read-only transaction
func (dm *datastoreManager) BeginReadOnlyTransaction(ctx context.Context) (*sql.Tx, error) {
	db, err := dm.GetReadOnlyConnection(ctx)
	if err != nil {
		return nil, err
	}

	if dm.metricsCollector != nil {
		dm.metricsCollector.RecordTransactionStarted()
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		if dm.metricsCollector != nil {
			dm.metricsCollector.RecordConnectionError(err)
		}
		return nil, fmt.Errorf("failed to begin read-only transaction: %w", err)
	}

	return tx, nil
}

// GetConnectionPoolStats returns connection pool statistics
func (dm *datastoreManager) GetConnectionPoolStats() ConnectionPoolStats {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	if !dm.initialized || dm.writeDB == nil {
		return ConnectionPoolStats{}
	}

	stats := dm.writeDB.Stats()
	
	return ConnectionPoolStats{
		OpenConnections:    stats.OpenConnections,
		InUse:             stats.InUse,
		Idle:              stats.Idle,
		WaitCount:         stats.WaitCount,
		WaitDuration:      stats.WaitDuration,
		MaxIdleClosed:     stats.MaxIdleClosed,
		MaxIdleTimeClosed: stats.MaxIdleTimeClosed,
		MaxLifetimeClosed: stats.MaxLifetimeClosed,
	}
}

// Close closes all database connections
func (dm *datastoreManager) Close() error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if !dm.initialized {
		return nil
	}

	var errors []error

	// Stop health monitoring
	if dm.healthChecker != nil {
		dm.healthChecker.StopHealthMonitoring()
	}

	// Close write connection
	if dm.writeDB != nil {
		if err := dm.writeDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close write database: %w", err))
		}
		dm.writeDB = nil
	}

	// Close read connection if it's different from write connection
	if dm.readDB != nil && dm.readDB != dm.writeDB {
		if err := dm.readDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close read database: %w", err))
		}
		dm.readDB = nil
	}

	dm.initialized = false

	if dm.logger != nil {
		dm.logger.Info("Datastore manager closed")
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors occurred while closing datastore: %v", errors)
	}

	return nil
}
