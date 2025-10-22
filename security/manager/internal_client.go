package manager

import (
	"fmt"
)

// JwtClient gets the authenticated jwt client if configured at startup.
func (s *managerImpl) JwtClient() map[string]any {
	return s.jwtClient
}

// SetJwtClient sets the authenticated jwt client.
func (s *managerImpl) SetJwtClient(jwtCli map[string]any) {
	s.jwtClient = jwtCli
}

// JwtClientID gets the authenticated JWT client ID if configured at startup.
func (s *managerImpl) JwtClientID() string {
	clientID, ok := s.jwtClient["client_id"].(string)
	if ok {
		return clientID
	}

	clientID = s.cfg.GetOauth2ServiceClientID()
	if clientID != "" {
		return clientID
	}

	clientID = s.serviceName
	if s.serviceEnvironment != "" {
		clientID = fmt.Sprintf("%s_%s", s.serviceName, s.serviceEnvironment)
	}

	return clientID
}

// JwtClientSecret gets the authenticated jwt client if configured at startup.
func (s *managerImpl) JwtClientSecret() string {
	clientSecret, ok := s.jwtClient["client_secret"].(string)
	if ok {
		return clientSecret
	}
	if s.cfg != nil {
		return s.cfg.GetOauth2ServiceClientSecret()
	}
	return ""
}
