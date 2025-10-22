package frame

import (
	"context"

	"github.com/pitabwire/frame/localization"
)

// WithTranslation Option that helps to specify or override the configuration object of our service.
func WithTranslation(translationsFolder string, languages ...string) Option {
	return func(_ context.Context, s *Service) {
		s.localizationManager = localization.NewManager(translationsFolder, languages...)
	}
}

func (s *Service) Localization() localization.Manager {
	return s.localizationManager
}
