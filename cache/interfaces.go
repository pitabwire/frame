package cache

type Manager interface {
	AddCache(name string, cache RawCache)
	GetRawCache(name string) (RawCache, bool)
	RemoveCache(name string) error
	Close() error
}
