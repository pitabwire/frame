package frameserver

import (
	"os"

	"github.com/pitabwire/frame/internal/common"
)

func TLSEnabled(s common.Service) bool {
	config, ok := s.Config().(common.ConfigurationTLS)
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
