package frame

import (
	"os"
)

func (s *Service) TLSEnabled() bool {
	config, ok := s.Config().(ConfigurationTLS)
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
