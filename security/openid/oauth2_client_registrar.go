package openid

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

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

func NewClientRegistrar(
	serviceName, serviceEnv string,
	cfg config.ConfigurationOAUTH2,
	cli client.Manager,
) security.Oauth2ClientRegistrar {
	return &clientRegistrar{
		serviceName:        serviceName,
		serviceEnvironment: serviceEnv,
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
			clientID += "_" + s.serviceEnvironment
		}
	}

	clientSecret := oauth2Config.GetOauth2ServiceClientSecret()

	oauth2Audience := oauth2Config.GetOauth2ServiceAudience()

	if oauth2ServiceAdminHost != "" {
		jwtClient, err := s.RegisterForJwtWithParams(ctx,
			oauth2ServiceAdminHost, s.serviceName, clientID, clientSecret,
			ConstSystemScopeInternal, oauth2Audience, map[string]string{})
		if err != nil {
			return err
		}

		iClientHolder.SetJwtClient(clientID, clientSecret, jwtClient)
	}

	return nil
}

// RegisterForJwtWithParams registers for JWT with the given parameters. This is useful for situations where one may need to register external applications for access token generation.
func (s *clientRegistrar) RegisterForJwtWithParams(ctx context.Context,
	oauth2ServiceAdminHost string, clientName string, clientID string, clientSecret string,
	scope string, audienceList []string, metadata map[string]string) (map[string]any, error) {
	oauth2AdminURI := fmt.Sprintf("%s/admin/clients", oauth2ServiceAdminHost)
	oauth2AdminIDURI := fmt.Sprintf("%s/%s", oauth2AdminURI, clientID)

	resp, err := s.invoker.Invoke(ctx, http.MethodGet, oauth2AdminIDURI, nil, nil)
	if err != nil {
		util.Log(ctx).WithError(err).Error("could not get existing clients")
		return nil, err
	}

	if resp.StatusCode != http.StatusNotFound && (resp.StatusCode > 299 || resp.StatusCode < 200) {
		body, _ := resp.ToContent(ctx)
		util.Log(ctx).
			WithField("status", resp.StatusCode).
			WithField("result", string(body)).
			Error(" invalid response from oauth2 server")

		return nil, fmt.Errorf("invalid existing clients check response : %s", body)
	}

	if resp.StatusCode != http.StatusNotFound {
		var existingClient map[string]any
		err = resp.Decode(ctx, &existingClient)
		if err != nil {
			util.Log(ctx).WithError(err).Error("could not unmarshal existing clients")
			return nil, err
		}

		return existingClient, nil
	}

	_ = resp.Close()

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

	resp, err = s.invoker.Invoke(ctx, http.MethodPost, oauth2AdminURI, payload, nil)
	if err != nil {
		util.Log(ctx).WithError(err).Error("could not create a new client")
		return nil, err
	}

	if resp.StatusCode > 299 || resp.StatusCode < 200 {
		body, _ := resp.ToContent(ctx)
		util.Log(ctx).
			WithField("status", resp.StatusCode).
			WithField("result", string(body)).
			Error(" invalid response from server")

		return nil, fmt.Errorf("invalid registration response : %s", body)
	}

	var newClient map[string]any
	err = resp.Decode(ctx, &newClient)
	if err != nil {
		util.Log(ctx).WithError(err).Error("could not un marshal new client")
		return nil, err
	}

	return newClient, nil
}

// UnRegisterForJwt utilizing client id we de register external applications for access token generation.
func (s *clientRegistrar) UnRegisterForJwt(ctx context.Context,
	oauth2ServiceAdminHost string, clientID string) error {
	oauth2AdminURI := fmt.Sprintf("%s/admin/clients/%s", oauth2ServiceAdminHost, clientID)

	resp, err := s.invoker.Invoke(
		ctx,
		http.MethodDelete,
		oauth2AdminURI,
		make(map[string]any),
		nil,
	)
	if err != nil {
		util.Log(ctx).
			WithError(err).
			Error(" invalid response from server")
		return err
	}

	_ = resp.Close()

	return nil
}
