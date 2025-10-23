package frame

import (
	"context"

	"github.com/pitabwire/frame/cache"
)

// WithCacheManager adds a cache manager to the service.
func WithCacheManager() Option {
	return func(_ context.Context, s *Service) {
		if s.cacheManager == nil {
			s.cacheManager = cache.NewManager()

			// Register cleanup method
			s.AddCleanupMethod(func(_ context.Context) {
				if s.cacheManager != nil {
					_ = s.cacheManager.Close()
				}
			})
		}
	}
}

// WithCache adds a raw cache with the given name to the service.
func WithCache(name string, rawCache cache.RawCache) Option {
	return func(ctx context.Context, s *Service) {
		// Ensure cache manager is initialized
		if s.cacheManager == nil {
			WithCacheManager()(ctx, s)
		}

		// Add cache
		s.cacheManager.AddCache(name, rawCache)
	}
}

// WithInMemoryCache adds an in-memory cache with the given name.
func WithInMemoryCache(name string) Option {
	return WithCache(name, cache.NewInMemoryCache())
}

// CacheManager returns the service's cache manager.
func (s *Service) CacheManager() cache.Manager {
	return s.cacheManager
}

// GetRawCache is a convenience method to get a raw cache by name from the service.
func (s *Service) GetRawCache(name string) (cache.RawCache, bool) {
	if s.cacheManager == nil {
		return nil, false
	}
	return s.cacheManager.GetRawCache(name)
}
