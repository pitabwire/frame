package frame

import (
	"context"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/manager"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/tenancy"
	tenpg "github.com/pitabwire/frame/tenancy/postgres"
)

// WithDatastoreManager creates and initializes a datastore manager with the given options.
// This is the low-level function that should rarely be called directly - use WithDatastore instead.
func WithDatastoreManager() Option {
	return func(ctx context.Context, s *Service) {
		s.registerPlugin("datastore")

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
			// Forward all opts so adapter/provider overrides reach NewPool —
			// NewPool reads DialectAdapter and TenancyProvider, ignores the rest.
			dbPool = pool.NewPool(ctx, opts...)
			s.datastoreManager.AddPool(ctx, name, dbPool)
		}

		err := dbPool.AddConnection(ctx, opts...)
		if err != nil {
			s.AddStartupError(err)
		}
	}
}

// datastoreOptsFromConfig reads datastore-related configuration from the service
// and returns enriched pool options and whether migration should run.
func datastoreOptsFromConfig(s *Service, opts []pool.Option) ([]pool.Option, bool) {
	var connectionSlice []pool.Connection
	doMigrate := false

	traceCfg, ok := s.Config().(config.ConfigurationDatabaseTracing)
	if ok {
		opts = append(opts, pool.WithTraceConfig(traceCfg))
	}

	cfg, ok := s.Config().(config.ConfigurationDatabase)
	if ok {
		for _, primaryDBURL := range cfg.GetDatabasePrimaryHostURL() {
			connectionSlice = append(connectionSlice, pool.Connection{DSN: primaryDBURL, ReadOnly: false})
		}
		for _, replicaDBURL := range cfg.GetDatabaseReplicaHostURL() {
			connectionSlice = append(connectionSlice, pool.Connection{DSN: replicaDBURL, ReadOnly: true})
		}
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

	opts = append(opts, pool.WithConnections(connectionSlice))
	return opts, doMigrate
}

func WithDatastore(opts ...pool.Option) Option {
	return func(ctx context.Context, s *Service) {
		enrichedOpts, doMigrate := datastoreOptsFromConfig(s, opts)

		// Default to Postgres-RLS only if WithTenancyProvider was never
		// called. WithTenancyProvider(nil) explicitly disables enforcement.
		if !s.tenancyProviderSet {
			s.tenancyProvider = tenpg.New()
		}
		// Forward the provider through to the pool so it wires hooks before
		// any connection is opened. The pool also defaults internally, but
		// passing it explicitly makes the wiring visible.
		enrichedOpts = append(enrichedOpts, pool.WithTenancyProvider(s.tenancyProvider))

		// Create the manager if it doesn't exist
		dbManager := WithDatastoreManager()
		dbManager(ctx, s)

		dbConnectionOpts := WithDatastoreConnectionWithOptions(datastore.DefaultPoolName, enrichedOpts...)
		dbConnectionOpts(ctx, s)

		if doMigrate {
			// minor feature to automatically make a pool that can be used for db migrations
			enrichedOpts = append(enrichedOpts, pool.WithPreparedStatements(false))
			migrationOpts := WithDatastoreConnectionWithOptions(datastore.DefaultMigrationPoolName, enrichedOpts...)
			migrationOpts(ctx, s)
		}
	}
}

// WithTenancyProvider overrides the default Postgres-RLS tenancy
// provider that WithDatastore wires by default. Pass nil to disable
// tenancy enforcement entirely (useful for tests).
//
// May be passed before or after WithDatastore in the option list —
// WithDatastore resolves s.tenancyProvider at construction time, and
// the resolved provider flows through to pool.NewPool via opts.
func WithTenancyProvider(prov tenancy.Provider) Option {
	return func(_ context.Context, s *Service) {
		s.tenancyProvider = prov
		s.tenancyProviderSet = true
	}
}

// DatastoreManager returns the service's datastore manager.
func (s *Service) DatastoreManager() datastore.Manager {
	return s.datastoreManager
}

// TenancyProvider returns the tenancy provider wired for the default
// pool. Used by tests and diagnostics. May be nil when tenancy has
// been explicitly disabled via WithTenancyProvider(nil).
func (s *Service) TenancyProvider() tenancy.Provider {
	return s.tenancyProvider
}
