package pool

import (
	"time"

	"github.com/pitabwire/frame/config"
)

// Option configures database connection settings.
type Option func(*Options)

// Options holds Datastore connection configuration.
type Options struct {
	Connections []Connection

	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration

	PreferSimpleProtocol   bool
	SkipDefaultTransaction bool

	TraceConfig     config.ConfigurationDatabaseTracing
	InsertBatchSize int

	PreparedStatements bool
}

// WithConnection returns an Option to configure the database connection dsn.
// Multiple calls with the same DSN but different readOnly flags are supported.
func WithConnection(dsn string, readOnly bool) Option {
	return func(o *Options) {
		o.Connections = append(o.Connections, Connection{
			DSN:      dsn,
			ReadOnly: readOnly,
		})
	}
}

// WithConnections returns an Option to configure database connections from a slice.
// Supports adding multiple connections including the same DSN with different readOnly flags.
func WithConnections(connections []Connection) Option {
	return func(o *Options) {
		o.Connections = append(o.Connections, connections...)
	}
}

// WithMaxOpen returns an Option to configure the database connection max open connections.
func WithMaxOpen(maxOpen int) Option {
	return func(o *Options) {
		o.MaxOpen = maxOpen
	}
}

// WithMaxIdle returns an Option to configure the database connection max idle connections.
func WithMaxIdle(maxIdle int) Option {
	return func(o *Options) {
		o.MaxIdle = maxIdle
	}
}

// WithMaxLifetime returns an Option to configure the database connection max lifetime.
func WithMaxLifetime(maxLifetime time.Duration) Option {
	return func(o *Options) {
		o.MaxLifetime = maxLifetime
	}
}

// WithPreferSimpleProtocol returns an Option to configure the database connection prefer simple protocol.
func WithPreferSimpleProtocol(preferSimpleProtocol bool) Option {
	return func(o *Options) {
		o.PreferSimpleProtocol = preferSimpleProtocol
	}
}

// WithSkipDefaultTransaction returns an Option to configure the database connection skip default transaction.
func WithSkipDefaultTransaction(skipDefaultTransaction bool) Option {
	return func(o *Options) {
		o.SkipDefaultTransaction = skipDefaultTransaction
	}
}

// WithTraceConfig returns an Option to configure the database connection trace config.
func WithTraceConfig(traceConfig config.ConfigurationDatabaseTracing) Option {
	return func(o *Options) {
		o.TraceConfig = traceConfig
	}
}

// WithInsertBatchSize returns an Option to configure the database connection insert batch size.
func WithInsertBatchSize(insertBatchSize int) Option {
	return func(o *Options) {
		o.InsertBatchSize = insertBatchSize
	}
}

// WithPreparedStatements returns an Option to enable or disable the prepared statement cache.
func WithPreparedStatements(enabled bool) Option {
	return func(o *Options) {
		o.PreparedStatements = enabled
	}
}
