package frame

import (
	"context"
)

// WithConfig Option that helps to specify or override the configuration object of our service.
func WithConfig(config any) Option {
	return func(_ context.Context, s *Service) {
		s.configuration = config
	}
}

func (s *Service) Config() any {
	return s.configuration
}
