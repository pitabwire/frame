package frame

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const systemScope = "int_system"

// RegisterForJwt function hooks in jwt client registration to make sure service is authenticated.
func (s *Service) RegisterForJwt(ctx context.Context) error {
	oauth2Config, ok := s.Config().(ConfigurationOAUTH2)
	if ok {
		oauth2ServiceAdminHost := oauth2Config.GetOauth2ServiceAdminURI()

		clientID := oauth2Config.GetOauth2ServiceClientID()
		if clientID == "" {
			clientID = fmt.Sprintf("%s_%s", s.Name(), s.Environment())
		}

		clientSecret := oauth2Config.GetOauth2ServiceClientSecret()

		oauth2Audience := oauth2Config.GetOauth2ServiceAudience()

		if oauth2ServiceAdminHost != "" && clientSecret != "" {
			audienceList := strings.Split(oauth2Audience, ",")

			jwtClient, err := s.RegisterForJwtWithParams(ctx, oauth2ServiceAdminHost, s.Name(), clientID, clientSecret,
				systemScope, audienceList, map[string]string{})
			if err != nil {
				return err
			}

			s.jwtClient = jwtClient
			s.jwtClientSecret = clientSecret
		}
	}
	return nil
}

// RegisterForJwtWithParams registers for JWT with the given parameters. This is useful for situations where one may need to register external applications for access token generation.
func (s *Service) RegisterForJwtWithParams(ctx context.Context,
	oauth2ServiceAdminHost string, clientName string, clientID string, clientSecret string,
	scope string, audienceList []string, metadata map[string]string) (map[string]any, error) {
	oauth2AdminURI := fmt.Sprintf("%s/admin/clients", oauth2ServiceAdminHost)
	oauth2AdminIDUri := fmt.Sprintf("%s/%s", oauth2AdminURI, clientID)

	status, response, err := s.InvokeRestService(ctx, http.MethodGet, oauth2AdminIDUri, nil, nil)
	if err != nil {
		s.Log(ctx).WithError(err).Error("could not get existing clients")
		return nil, err
	}

	if status > 299 || status < 200 {
		s.Log(ctx).
			WithField("status", status).
			WithField("result", string(response)).
			Error(" invalid response from oauth2 server")

		return nil, fmt.Errorf("invalid existing clients check response : %s", response)
	}

	var existingClients []map[string]any
	err = json.Unmarshal(response, &existingClients)
	if err != nil {
		s.Log(ctx).WithError(err).WithField("payload", string(response)).
			Error("could not unmarshal existing clients")
		return nil, err
	}

	if len(existingClients) > 0 {
		return existingClients[0], nil
	}

	payload := map[string]any{
		"client_id":                  clientID,
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
		s.Log(ctx).WithError(err).Error("could not create a new client")
		return nil, err
	}

	if status > 299 || status < 200 {
		s.Log(ctx).
			WithField("status", status).
			WithField("result", string(response)).
			Error(" invalid response from server")

		return nil, fmt.Errorf("invalid registration response : %s", response)
	}

	var newClient map[string]any
	err = json.Unmarshal(response, &newClient)
	if err != nil {
		s.Log(ctx).WithError(err).Error("could not un marshal new client")
		return nil, err
	}

	return newClient, nil
}

// UnRegisterForJwt utilizing client id we de register external applications for access token generation.
func (s *Service) UnRegisterForJwt(ctx context.Context,
	oauth2ServiceAdminHost string, clientID string) error {
	oauth2AdminURI := fmt.Sprintf("%s%s/%s", oauth2ServiceAdminHost, "/admin/clients", clientID)

	status, result, err := s.InvokeRestService(ctx, http.MethodDelete, oauth2AdminURI, make(map[string]any), nil)
	if err != nil {
		s.Log(ctx).
			WithError(err).
			WithField("status", status).
			WithField("result", string(result)).
			Error(" invalid response from server")
		return err
	}
	return nil
}
