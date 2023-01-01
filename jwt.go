package frame

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const envOauth2ServiceClientSecret = "OAUTH2_SERVICE_CLIENT_SECRET"
const envOauth2ServiceAdminURI = "OAUTH2_SERVICE_ADMIN_URI"
const envOauth2Audience = "OAUTH2_SERVICE_AUDIENCE"

func (s *Service) registerForJwt(ctx context.Context) error {
	clientSecret := GetEnv(envOauth2ServiceClientSecret, "")
	if clientSecret == "" {
		return nil
	}

	return s.RegisterForJwtWithParams(ctx, s.name, s.name, clientSecret)
}

// RegisterForJwtWithParams registers the supplied details for ability to generate access tokens.
// This is useful for situations where one may need to register external applications for access token generation
func (s *Service) RegisterForJwtWithParams(ctx context.Context, clientName string, clientID string, clientSecret string) error {
	host := GetEnv(envOauth2ServiceAdminURI, "")
	if host == "" {
		return nil
	}

	audienceList := strings.Split(GetEnv(envOauth2Audience, ""), ",")

	oauth2AdminURI := fmt.Sprintf("%s%s", host, "/clients")
	oauth2AdminIDUri := fmt.Sprintf("%s/%s", oauth2AdminURI, s.name)

	status, _, err := s.InvokeRestService(ctx, http.MethodGet, oauth2AdminIDUri, make(map[string]interface{}), nil)
	if err != nil {
		return err
	}

	if status == 200 {
		return nil
	}

	payload := map[string]interface{}{
		"client_id":                  clientID,
		"client_name":                clientName,
		"client_secret":              clientSecret,
		"grant_types":                []string{"client_credentials"},
		"metadata":                   map[string]string{"cc_bot": "true"},
		"audience":                   audienceList,
		"token_endpoint_auth_method": "client_secret_post",
	}

	status, result, err := s.InvokeRestService(ctx, http.MethodPost, oauth2AdminURI, payload, nil)
	if err != nil {
		return err
	}

	if status > 299 || status < 200 {
		return fmt.Errorf(" invalid response status %d had message %s", status, string(result))
	}
	return nil
}
