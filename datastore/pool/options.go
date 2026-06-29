package pool

import (
	"time"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/datastore/dialect"
	"github.com/pitabwire/frame/v2/tenancy"
)

// Option configures database connection settings.
type Option func(*Options)

// Options holds Datastore connection configuration.
type Options struct {
	Connections []Connection

	MaxOpen     int
	MaxLifetime time.Duration

	PreferSimpleProtocol   bool
	SkipDefaultTransaction bool

	TraceConfig     config.ConfigurationDatabaseTracing
	InsertBatchSize int

	PreparedStatements bool

	// DialectAdapter and TenancyProvider are resolved by NewPool with
	// Postgres defaults when unset. TenancyProviderSet distinguishes
	// "use default" from "explicitly disabled (nil)".
	DialectAdapter     dialect.DialectAdapter
	TenancyProvider    tenancy.Provider
	TenancyProviderSet bool
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

// WithMaxIdle is intentionally a no-op. The dialect adapter forces
// MaxIdleConns=0 on the sql.DB so every connection release flows
// through the per-acquire / per-release hook chain (which the tenancy
// provider relies on for session-state cleanup). The option is kept
// to avoid breaking callers that still set it.
//
// Deprecated: setting MaxIdle has no effect.
func WithMaxIdle(_ int) Option {
	return func(*Options) {}
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

// WithDialectAdapter sets the database driver adapter for this pool.
// When omitted, the pool uses the Postgres adapter.
func WithDialectAdapter(adapter dialect.DialectAdapter) Option {
	return func(o *Options) {
		o.DialectAdapter = adapter
	}
}

// WithTenancyProvider sets the tenancy provider for this pool. When
// omitted, the pool uses the Postgres-RLS provider. Pass nil to
// disable tenancy enforcement (useful in unit tests that want raw
// database access).
func WithTenancyProvider(prov tenancy.Provider) Option {
	return func(o *Options) {
		o.TenancyProvider = prov
		o.TenancyProviderSet = true
	}
}
