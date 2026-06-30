package cache

import "context"

type Manager interface {
	AddCache(name string, cache RawCache)
	GetRawCache(name string) (RawCache, bool)
	RemoveCache(name string) error
	Shutdown(ctx context.Context) error
}
