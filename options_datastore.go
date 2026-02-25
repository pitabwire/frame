package frame

import (
	"context"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/manager"
	"github.com/pitabwire/frame/datastore/pool"
)

// WithDatastoreManager creates and initializes a datastore manager with the given options.
// This is the low-level function that should rarely be called directly - use WithDatastore instead.
func WithDatastoreManager() Option {
	return func(ctx context.Context, s *Service) {
		if s.datastoreManager == nil {
			var err error
			s.datastoreManager, err = manager.NewManager(ctx)
			if err != nil {
				s.AddStartupError(err)
				return
			}

			// Register cleanup method
			s.AddCleanupMethod(func(_ context.Context) {
				if s.datastoreManager != nil {
					s.datastoreManager.Close(ctx)
				}
			})
		}
	}
}

// WithDatastoreConnection Option method to store a connection that will be utilized when connecting to the database.
func WithDatastoreConnection(postgresqlConnection string, readOnly bool) Option {
	return WithDatastoreConnectionWithName(datastore.DefaultPoolName, postgresqlConnection, readOnly)
}

func WithDatastoreConnectionWithName(
	name string,
	postgresqlConnection string,
	readOnly bool,
	opts ...pool.Option,
) Option {
	return WithDatastoreConnectionWithOptions(
		name,
		append(opts, pool.WithConnection(postgresqlConnection, readOnly))...)
}

func WithDatastoreConnectionWithOptions(name string, opts ...pool.Option) Option {
	return func(ctx context.Context, s *Service) {
		// First ensure the manager exists
		dbManager := WithDatastoreManager()
		dbManager(ctx, s)

		dbPool := s.datastoreManager.GetPool(ctx, name)
		if dbPool == nil {
			dbPool = pool.NewPool(ctx)
			s.datastoreManager.AddPool(ctx, name, dbPool)
		}

		err := dbPool.AddConnection(ctx, opts...)
		if err != nil {
			s.AddStartupError(err)
		}
	}
}

func WithDatastore(opts ...pool.Option) Option {
	return func(ctx context.Context, s *Service) {
		var connectionSlice []pool.Connection
		doMigrate := false

		traceCfg, ok := s.Config().(config.ConfigurationDatabaseTracing)
		if ok {
			opts = append(opts, pool.WithTraceConfig(traceCfg))
		}

		cfg, ok := s.Config().(config.ConfigurationDatabase)
		if ok {
			// Add connections from config if available
			for _, primaryDBURL := range cfg.GetDatabasePrimaryHostURL() {
				connectionSlice = append(connectionSlice, pool.Connection{DSN: primaryDBURL, ReadOnly: false})
			}

			for _, replicaDBURL := range cfg.GetDatabaseReplicaHostURL() {
				connectionSlice = append(connectionSlice, pool.Connection{DSN: replicaDBURL, ReadOnly: true})
			}

			// Apply operational pool tuning from configuration.
			if cfg.GetMaxOpenConnections() > 0 {
				opts = append(opts, pool.WithMaxOpen(cfg.GetMaxOpenConnections()))
			}
			if cfg.GetMaxIdleConnections() > 0 {
				opts = append(opts, pool.WithMaxIdle(cfg.GetMaxIdleConnections()))
			}
			if cfg.GetMaxConnectionLifeTimeInSeconds() > 0 {
				opts = append(opts, pool.WithMaxLifetime(cfg.GetMaxConnectionLifeTimeInSeconds()))
			}

			doMigrate = cfg.DoDatabaseMigrate()
		}

		// Add connections from config (if any exist)
		opts = append(opts, pool.WithConnections(connectionSlice))

		// Create the manager if it doesn't exist
		dbManager := WithDatastoreManager()
		dbManager(ctx, s)

		dbConnectionOpts := WithDatastoreConnectionWithOptions(datastore.DefaultPoolName, opts...)
		dbConnectionOpts(ctx, s)

		if doMigrate {
			// minor feature to automatically make a pool that can be used for db migrations
			opts = append(opts, pool.WithPreparedStatements(false))
			migrationOpts := WithDatastoreConnectionWithOptions(datastore.DefaultMigrationPoolName, opts...)
			migrationOpts(ctx, s)
		}
	}
}

// DatastoreManager returns the service's datastore manager.
func (s *Service) DatastoreManager() datastore.Manager {
	return s.datastoreManager
}
