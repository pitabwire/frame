package cache

import (
	"time"

	"github.com/pitabwire/frame/data"
)

// Option configures database connection settings.
type Option func(*Options)

// Options holds Datastore connection configuration.
type Options struct {
	DSN    data.DSN
	Name   string
	MaxAge time.Duration
}

func WithDSN(dsn data.DSN) Option {
	return func(o *Options) {
		o.DSN = dsn
	}
}

func WithName(name string) Option {
	return func(o *Options) {
		o.Name = name
	}
}

// WithMaxAge returns an Option to configure the max age of the cache.
func WithMaxAge(maxAge time.Duration) Option {
	return func(o *Options) {
		o.MaxAge = maxAge
	}
}
