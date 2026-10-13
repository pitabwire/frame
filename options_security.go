package frame

import (
	"github.com/pitabwire/frame/v3/security"
)

func (s *Service) SecurityManager() security.Manager {
	return s.securityManager
}
