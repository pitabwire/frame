package frame

import (
	"context"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/manager"
	"github.com/pitabwire/frame/datastore/pool"
)

// WithDatastoreManager adds a cache manager to the service.
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

// WithDatastoreConnection Option method to store a connection that will be utilized when connecting to the database.
func WithDatastoreConnection(postgresqlConnection string, readOnly bool) Option {
	return WithDatastoreConnectionWithName(datastore.DefaultPoolName, postgresqlConnection, readOnly)
}

func WithDatastoreConnectionWithName(
	name string,
	postgresqlConnection string,
	readOnly bool,
	opts ...datastore.Option,
) Option {
	return WithDatastoreConnectionWithOptions(
		name,
		append(opts, datastore.WithConnection(postgresqlConnection, readOnly))...)
}

func WithDatastoreConnectionWithOptions(name string, opts ...datastore.Option) Option {
	return func(ctx context.Context, s *Service) {
		dsOpts := datastore.Options{
			Name: datastore.DefaultPoolName,
		}

		for _, opt := range opts {
			opt(&dsOpts)
		}

		if len(dsOpts.DSNMap) > 0 {
			dbPool := s.datastoreManager.GetPool(ctx, name)
			if dbPool == nil {
				dbPool = pool.NewPool(ctx)
				s.datastoreManager.AddPool(ctx, name, dbPool)
			}

			for dsn, readOnly := range dsOpts.DSNMap {
				err := dbPool.AddConnection(ctx, dsn, readOnly, dsOpts.PoolOptions...)
				if err != nil {
					util.Log(ctx).WithError(err).WithField("dsn", dsn).Fatal("error initiating datastore connection")
				}
			}
		}
	}
}

func WithDatastore(opts ...datastore.Option) Option {
	return func(ctx context.Context, s *Service) {
		cfg, ok := s.Config().(config.ConfigurationDatabase)
		if !ok {
			s.Log(ctx).Warn("configuration object not of type : ConfigurationDatabase")
			return
		}

		connectionMap := map[string]bool{}
		for _, primaryDBURL := range cfg.GetDatabasePrimaryHostURL() {
			connectionMap[primaryDBURL] = false
		}

		for _, replicaDBURL := range cfg.GetDatabaseReplicaHostURL() {
			connectionMap[replicaDBURL] = true
		}

		opts = append(opts, datastore.WithConnections(connectionMap))

		dbManager := WithDatastoreManager(opts...)
		dbManager(ctx, s)
	}
}

// DatastoreManager returns the service's datastore manager.
func (s *Service) DatastoreManager() datastore.Manager {
	return s.datastoreManager
}
