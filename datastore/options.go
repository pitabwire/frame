package datastore

import (
	"github.com/pitabwire/frame/datastore/pool"
)

// Option configures database connection settings.
type Option func(*Options)

// Options holds Datastore connection configuration.
type Options struct {
	Name        string
	DSNMap      map[string]bool
	ReadOnly    bool
	PoolOptions []pool.Option
}

// WithName returns an Option to configure the database connection name.
func WithName(name string) Option {
	return func(o *Options) {
		o.Name = name
	}
}

// WithConnection returns an Option to configure the database connection dsn.
func WithConnection(dsn string, readOnly bool) Option {
	return func(o *Options) {
		if o.DSNMap == nil {
			o.DSNMap = make(map[string]bool)
		}

		o.DSNMap[dsn] = readOnly
	}
}

// WithConnections returns an Option to configure database connections.
func WithConnections(dsns map[string]bool) Option {
	return func(o *Options) {

		if o.DSNMap == nil {
			o.DSNMap = dsns
			return
		}

		for k, v := range dsns {
			o.DSNMap[k] = v
		}
	}
}

// WithPoolOptions returns an Option to configure the database connection pool options.
func WithPoolOptions(poolOptions []pool.Option) Option {
	return func(o *Options) {
		o.PoolOptions = poolOptions
	}
}
