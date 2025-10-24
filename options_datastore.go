package frame

import (
	"context"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/manager"
)

// WithDatastoreManager creates and initializes a datastore manager with the given options.
// This is the low-level function that should rarely be called directly - use WithDatastore instead.
func WithDatastoreManager(opts ...datastore.Option) Option {
	return func(ctx context.Context, s *Service) {
		if s.datastoreManager == nil {
			var err error
			s.datastoreManager, err = manager.NewManager(ctx, opts...)
			if err != nil {
				util.Log(ctx).WithError(err).Fatal("error initiating datastore manager")
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

// WithDatastoreConnection adds a single database connection to the default pool.
// Deprecated: Use WithDatastore(datastore.WithConnection(...)) instead for consistency.
func WithDatastoreConnection(postgresqlConnection string, readOnly bool) Option {
	return WithDatastore(datastore.WithConnection(postgresqlConnection, readOnly))
}

func WithDatastore(opts ...datastore.Option) Option {
	return func(ctx context.Context, s *Service) {
		connectionMap := map[string]bool{}

		cfg, ok := s.Config().(config.ConfigurationDatabase)
		if ok {
			// Add connections from config if available
			for _, primaryDBURL := range cfg.GetDatabasePrimaryHostURL() {
				connectionMap[primaryDBURL] = false
			}

			for _, replicaDBURL := range cfg.GetDatabaseReplicaHostURL() {
				connectionMap[replicaDBURL] = true
			}
		}

		// Add connections from config (if any exist)
		if len(connectionMap) > 0 {
			opts = append(opts, datastore.WithConnections(connectionMap))
		}

		// First ensure the manager exists
		dbManager := WithDatastoreManager(opts...)
		dbManager(ctx, s)

	}
}

// DatastoreManager returns the service's datastore manager.
func (s *Service) DatastoreManager() datastore.Manager {
	return s.datastoreManager
}
