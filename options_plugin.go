package frame

import "context"

// WithPlugin wraps an option and registers its name for introspection.
func WithPlugin(name string, opt Option) Option {
	return func(ctx context.Context, s *Service) {
		if name != "" {
			s.registerPlugin(name)
		}
		if opt != nil {
			opt(ctx, s)
		}
	}
}
