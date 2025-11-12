package openid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

const ConstSystemScopeInternal = "system_int"
const ConstSystemScopeExternal = "system_ext"

type clientRegistrar struct {
	serviceName        string
	serviceEnvironment string
	cfg                config.ConfigurationOAUTH2

	invoker client.Manager
}

func NewClientRegistrar(serviceName, serviceEnvironment string,
	cfg config.ConfigurationOAUTH2, cli client.Manager) security.Oauth2ClientRegistrar {
	return &clientRegistrar{
		serviceName:        serviceName,
		serviceEnvironment: serviceEnvironment,
		cfg:                cfg,
		invoker:            cli,
	}
}

// RegisterForJwt function hooks in jwt client registration to make sure service is authenticated.
func (s *clientRegistrar) RegisterForJwt(ctx context.Context, iClientHolder security.InternalOauth2ClientHolder) error {
	if s.cfg == nil {
		return nil
	}
	oauth2Config := s.cfg
	oauth2ServiceAdminHost := oauth2Config.GetOauth2ServiceAdminURI()

	clientID := oauth2Config.GetOauth2ServiceClientID()
	if clientID == "" {
		clientID = s.serviceName
		if s.serviceEnvironment != "" {
			clientID = fmt.Sprintf("%s_%s", s.serviceName, s.serviceEnvironment)
		}
	}

	clientSecret := oauth2Config.GetOauth2ServiceClientSecret()

	oauth2Audience := oauth2Config.GetOauth2ServiceAudience()

	if oauth2ServiceAdminHost != "" && clientSecret != "" {
		audienceList := strings.Split(oauth2Audience, ",")

		jwtClient, err := s.RegisterForJwtWithParams(ctx,
			oauth2ServiceAdminHost, s.serviceName, clientID, clientSecret,
			ConstSystemScopeInternal, audienceList, map[string]string{})
		if err != nil {
			return err
		}

		iClientHolder.SetJwtClient(jwtClient)
	}

	return nil
}

// RegisterForJwtWithParams registers for JWT with the given parameters. This is useful for situations where one may need to register external applications for access token generation.
func (s *clientRegistrar) RegisterForJwtWithParams(ctx context.Context,
	oauth2ServiceAdminHost string, clientName string, clientID string, clientSecret string,
	scope string, audienceList []string, metadata map[string]string) (map[string]any, error) {
	oauth2AdminURI := fmt.Sprintf("%s/admin/clients", oauth2ServiceAdminHost)
	oauth2AdminIDUri := fmt.Sprintf("%s/%s", oauth2AdminURI, clientID)

	status, response, err := s.invoker.Invoke(ctx, http.MethodGet, oauth2AdminIDUri, nil, nil)
	if err != nil {
		util.Log(ctx).WithError(err).Error("could not get existing clients")
		return nil, err
	}
	if status != http.StatusNotFound && (status > 299 || status < 200) {
		util.Log(ctx).
			WithField("status", status).
			WithField("result", string(response)).
			Error(" invalid response from oauth2 server")

		return nil, fmt.Errorf("invalid existing clients check response : %s", response)
	}

	if status != http.StatusNotFound {
		var existingClient map[string]any
		err = json.Unmarshal(response, &existingClient)
		if err != nil {
			util.Log(ctx).WithError(err).WithField("payload", string(response)).
				Error("could not unmarshal existing clients")
			return nil, err
		}

		return existingClient, nil
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

	status, response, err = s.invoker.Invoke(ctx, http.MethodPost, oauth2AdminURI, payload, nil)
	if err != nil {
		util.Log(ctx).WithError(err).Error("could not create a new client")
		return nil, err
	}

	if status > 299 || status < 200 {
		util.Log(ctx).
			WithField("status", status).
			WithField("result", string(response)).
			Error(" invalid response from server")

		return nil, fmt.Errorf("invalid registration response : %s", response)
	}

	var newClient map[string]any
	err = json.Unmarshal(response, &newClient)
	if err != nil {
		util.Log(ctx).WithError(err).Error("could not un marshal new client")
		return nil, err
	}

	return newClient, nil
}

// UnRegisterForJwt utilizing client id we de register external applications for access token generation.
func (s *clientRegistrar) UnRegisterForJwt(ctx context.Context,
	oauth2ServiceAdminHost string, clientID string) error {
	oauth2AdminURI := fmt.Sprintf("%s%s/%s", oauth2ServiceAdminHost, "/admin/clients", clientID)

	status, result, err := s.invoker.Invoke(
		ctx,
		http.MethodDelete,
		oauth2AdminURI,
		make(map[string]any),
		nil,
	)
	if err != nil {
		util.Log(ctx).
			WithError(err).
			WithField("status", status).
			WithField("result", string(result)).
			Error(" invalid response from server")
		return err
	}
	return nil
}
