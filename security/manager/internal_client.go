package manager

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

	return ""
}

// JwtClientSecret gets the authenticated jwt client if configured at startup.
func (s *managerImpl) JwtClientSecret() string {
	clientSecret, ok := s.jwtClient["client_secret"].(string)
	if ok {
		return clientSecret
	}
	return ""
}
