package frame

import (
	"net/http"
)

// HTTPClient obtains an instrumented http client for making appropriate calls downstream.
func (s *Service) HTTPClient() *http.Client {
	return s.client
}
