package frame

import (
	"github.com/pitabwire/frame/security"
)

func (s *Service) SecurityManager() security.Manager {
	return s.securityManager
}
