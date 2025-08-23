package framedata

import (
	"context"
	"database/sql"
	"time"
)

// DatastoreManager defines the contract for datastore management functionality
type DatastoreManager interface {
	// Connection management
	GetConnection(ctx context.Context) (*sql.DB, error)
	GetReadOnlyConnection(ctx context.Context) (*sql.DB, error)
	IsHealthy(ctx context.Context) bool
	
	// Transaction management
	BeginTransaction(ctx context.Context) (*sql.Tx, error)
	BeginReadOnlyTransaction(ctx context.Context) (*sql.Tx, error)
	
	// Connection pool management
	GetConnectionPoolStats() ConnectionPoolStats
	
	// Initialization and cleanup
	Initialize(ctx context.Context) error
	Close() error
}

// ConnectionPoolStats provides statistics about the connection pool
type ConnectionPoolStats struct {
	OpenConnections     int           // Number of established connections both in use and idle
	InUse              int           // Number of connections currently in use
	Idle               int           // Number of idle connections
	WaitCount          int64         // Total number of connections waited for
	WaitDuration       time.Duration // Total time blocked waiting for a new connection
	MaxIdleClosed      int64         // Total number of connections closed due to SetMaxIdleConns
	MaxIdleTimeClosed  int64         // Total number of connections closed due to SetConnMaxIdleTime
	MaxLifetimeClosed  int64         // Total number of connections closed due to SetConnMaxLifetime
}

// Config defines the configuration interface for datastore functionality
type Config interface {
	// Database connection configuration
	GetDatabaseURL() string
	GetDatabaseReadOnlyURL() string
	GetDatabaseDriver() string
	
	// Connection pool configuration
	GetDatabaseMaxOpenConnections() int
	GetDatabaseMaxIdleConnections() int
	GetDatabaseConnectionMaxLifetime() time.Duration
	GetDatabaseConnectionMaxIdleTime() time.Duration
	
	// Health check configuration
	GetDatabaseHealthCheckInterval() time.Duration
	GetDatabaseHealthCheckTimeout() time.Duration
	
	// Migration configuration
	GetDatabaseMigrationsPath() string
	GetDatabaseMigrationsTable() string
	
	// Security configuration
	IsRunSecurely() bool
}

// Migrator defines the interface for database migrations
type Migrator interface {
	// Migration operations
	Migrate(ctx context.Context) error
	MigrateUp(ctx context.Context, steps int) error
	MigrateDown(ctx context.Context, steps int) error
	
	// Migration status
	GetAppliedMigrations(ctx context.Context) ([]Migration, error)
	GetPendingMigrations(ctx context.Context) ([]Migration, error)
	
	// Migration validation
	ValidateMigrations(ctx context.Context) error
}

// Migration type moved to common.go to avoid duplication

// Repository defines the base interface for data repositories
type Repository interface {
	// Basic CRUD operations
	Create(ctx context.Context, entity interface{}) error
	GetByID(ctx context.Context, id interface{}) (interface{}, error)
	Update(ctx context.Context, entity interface{}) error
	Delete(ctx context.Context, id interface{}) error
	
	// Bulk operations
	CreateBatch(ctx context.Context, entities []interface{}) error
	UpdateBatch(ctx context.Context, entities []interface{}) error
	DeleteBatch(ctx context.Context, ids []interface{}) error
	
	// Query operations
	Find(ctx context.Context, filter interface{}) ([]interface{}, error)
	FindOne(ctx context.Context, filter interface{}) (interface{}, error)
	Count(ctx context.Context, filter interface{}) (int64, error)
	
	// Transaction support
	WithTransaction(tx *sql.Tx) Repository
}

// SearchProvider defines the interface for search functionality
type SearchProvider interface {
	// Search operations
	Search(ctx context.Context, query SearchQuery) (*SearchResult, error)
	SearchWithPagination(ctx context.Context, query SearchQuery, paginator Paginator) (*SearchResult, error)
	
	// Index management
	CreateIndex(ctx context.Context, indexName string, fields []string) error
	DropIndex(ctx context.Context, indexName string) error
	
	// Search capabilities
	SupportsFullTextSearch() bool
	SupportsFieldSearch() bool
	SupportsFacetedSearch() bool
}

// SearchQuery represents a search query
type SearchQuery struct {
	Query      string                 `json:"query"`
	Fields     []string               `json:"fields,omitempty"`
	Filters    map[string]interface{} `json:"filters,omitempty"`
	SortBy     []SortField            `json:"sort_by,omitempty"`
	ProfileID  interface{}            `json:"profile_id,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
	Offset     int                    `json:"offset,omitempty"`
}

// SortField represents a field to sort by
type SortField struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// SearchResult represents search results
type SearchResult struct {
	Items      []interface{} `json:"items"`
	Total      int64         `json:"total"`
	Took       time.Duration `json:"took"`
	HasMore    bool          `json:"has_more"`
	NextOffset int           `json:"next_offset,omitempty"`
}

// Paginator defines the interface for pagination
type Paginator interface {
	// Pagination control
	CanLoad() bool
	Stop()
	
	// Pagination parameters
	GetLimit() int
	GetOffset() int
	GetNextOffset() int
	
	// Pagination state
	IsComplete() bool
	GetLoadedCount() int64
}

// Logger defines the logging interface needed by the datastore module
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}

// MetricsCollector defines the interface for collecting datastore metrics
type MetricsCollector interface {
	// Connection metrics
	RecordConnectionAcquired(duration time.Duration)
	RecordConnectionReleased()
	RecordConnectionError(err error)
	
	// Query metrics
	RecordQueryExecuted(query string, duration time.Duration)
	RecordQueryError(query string, err error)
	
	// Transaction metrics
	RecordTransactionStarted()
	RecordTransactionCommitted(duration time.Duration)
	RecordTransactionRolledBack(duration time.Duration)
}

// HealthChecker defines the interface for health checking
type HealthChecker interface {
	// Health check operations
	CheckHealth(ctx context.Context) HealthStatus
	
	// Health monitoring
	StartHealthMonitoring(ctx context.Context, interval time.Duration) error
	StopHealthMonitoring()
	
	// Health status
	IsHealthy() bool
	GetLastHealthCheck() time.Time
}

// HealthStatus represents the health status of the datastore
type HealthStatus struct {
	Healthy        bool          `json:"healthy"`
	LastCheck      time.Time     `json:"last_check"`
	ResponseTime   time.Duration `json:"response_time"`
	Error          string        `json:"error,omitempty"`
	ConnectionPool ConnectionPoolStats `json:"connection_pool"`
}
