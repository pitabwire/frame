package frame

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// RegisterForJwtWithParams registers the supplied details for ability to generate access tokens.
// This is useful for situations where one may need to register external applications for access token generation
func (s *Service) RegisterForJwtWithParams(ctx context.Context,
	oauth2ServiceAdminHost string, clientName string, clientSecret string,
	scope string, audienceList []string, metadata map[string]string) (map[string]interface{}, error) {
	oauth2AdminURI := fmt.Sprintf("%s%s", oauth2ServiceAdminHost, "/admin/clients")
	oauth2AdminIDUri := fmt.Sprintf("%s?client_name=%s", oauth2AdminURI, url.QueryEscape(clientName))

	status, response, err := s.InvokeRestService(ctx, http.MethodGet, oauth2AdminIDUri, nil, nil)
	if err != nil {
		s.L().WithError(err).Error("could not get existing clients")
		return nil, err
	}

	if status > 299 || status < 200 {
		s.L().WithContext(ctx).
			WithField("status", status).
			WithField("result", string(response)).
			Error(" invalid response from oauth2 server")

		return nil, fmt.Errorf("invalid existing clients check response : %s", response)
	}

	var existingClients []map[string]interface{}
	err = json.Unmarshal(response, &existingClients)
	if err != nil {
		s.L().WithError(err).WithField("payload", string(response)).
			Error("could not unmarshal existing clients")
		return nil, err
	}

	if len(existingClients) > 0 {
		return existingClients[0], nil
	}

	metadata["cc_bot"] = "true"
	payload := map[string]interface{}{
		"client_name":                url.QueryEscape(clientName),
		"client_secret":              url.QueryEscape(clientSecret),
		"grant_types":                []string{"client_credentials"},
		"metadata":                   metadata,
		"audience":                   audienceList,
		"token_endpoint_auth_method": "client_secret_post",
	}

	if scope != "" {
		payload["scope"] = scope
	}

	status, response, err = s.InvokeRestService(ctx, http.MethodPost, oauth2AdminURI, payload, nil)
	if err != nil {
		s.L().WithError(err).Error("could not create a new client")
		return nil, err
	}

	if status > 299 || status < 200 {
		s.L().WithContext(ctx).
			WithField("status", status).
			WithField("result", string(response)).
			Error(" invalid response from server")

		return nil, fmt.Errorf("invalid registration response : %s", response)
	}

	var newClient map[string]interface{}
	err = json.Unmarshal(response, &newClient)
	if err != nil {
		s.L().WithError(err).Error("could not un marshal new client")
		return nil, err
	}

	return newClient, nil
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
