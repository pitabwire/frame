package frame

import (
	"github.com/pitabwire/frame/v2/security"
)

func (s *Service) SecurityManager() security.Manager {
	return s.securityManager
}
