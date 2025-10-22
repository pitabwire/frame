package frame

import (
	"os"

	config2 "github.com/pitabwire/frame/config"
)

func (s *Service) TLSEnabled() bool {
	config, ok := s.Config().(config2.ConfigurationTLS)
	if !ok {
		return false
	}

	if _, err := os.Stat(config.TLSCertPath()); err != nil {
		return false
	}

	if _, err := os.Stat(config.TLSCertKeyPath()); err != nil {
		return false
	}
	return true
}
