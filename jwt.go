package frame

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// RegisterForJwtWithParams registers the supplied details for ability to generate access tokens.
// This is useful for situations where one may need to register external applications for access token generation
func (s *Service) RegisterForJwtWithParams(ctx context.Context,
	oauth2ServiceAdminHost string, clientName string, clientID string, clientSecret string,
	scope string, audienceList []string, metadata map[string]string) error {
	oauth2AdminURI := fmt.Sprintf("%s%s", oauth2ServiceAdminHost, "/admin/clients")
	oauth2AdminIDUri := fmt.Sprintf("%s/%s", oauth2AdminURI, clientID)

	status, _, err := s.InvokeRestService(ctx, http.MethodGet, oauth2AdminIDUri, make(map[string]interface{}), nil)
	if err != nil {
		return err
	}

	if status == 200 {
		return nil
	}

	metadata["cc_bot"] = "true"

	payload := map[string]interface{}{
		"client_id":                  clientID,
		"client_name":                clientName,
		"client_secret":              clientSecret,
		"grant_types":                []string{"client_credentials"},
		"metadata":                   metadata,
		"audience":                   audienceList,
		"token_endpoint_auth_method": "client_secret_post",
	}

	if scope != "" {
		payload["scope"] = scope
	}

	status, result, err := s.InvokeRestService(ctx, http.MethodPost, oauth2AdminURI, payload, nil)
	if err != nil {
		return err
	}

	if status > 299 || status < 200 {
		s.L().WithContext(ctx).
			WithError(err).
			WithField("status", status).
			WithField("result", result).
			Error(" invalid response from server")

		return errors.New("invalid registration response")
	}
	return nil
}

// UnRegisterForJwt utilizing client id we de register external applications for access token generation
func (s *Service) UnRegisterForJwt(ctx context.Context,
	oauth2ServiceAdminHost string, clientID string) error {

	oauth2AdminURI := fmt.Sprintf("%s%s/%s", oauth2ServiceAdminHost, "/admin/clients", clientID)

	status, result, err := s.InvokeRestService(ctx, http.MethodDelete, oauth2AdminURI, make(map[string]interface{}), nil)
	if err != nil {
		s.L().WithContext(ctx).
			WithError(err).
			WithField("status", status).
			WithField("result", string(result)).
			Error(" invalid response from server")
		return err
	}
	return nil
}
