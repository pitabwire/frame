package security

import (
	"context"
)

type Oauth2ClientRegistrar interface {
	RegisterForJwt(ctx context.Context, iClientHolder InternalOauth2ClientHolder) error
	RegisterForJwtWithParams(ctx context.Context,
		oauth2ServiceAdminHost string, clientName string, clientID string, clientSecret string,
		scope string, audienceList []string, metadata map[string]string) (map[string]any, error)
	UnRegisterForJwt(ctx context.Context,
		oauth2ServiceAdminHost string, clientID string) error
}

type InternalOauth2ClientHolder interface {
	JwtClient() map[string]any
	SetJwtClient(clientID, clientSecret string, jwtCli map[string]any)
	JwtClientID() string
	JwtClientSecret() string
}

type Authenticator interface {
	Authenticate(ctx context.Context, jwtToken string, options ...AuthOption) (context.Context, error)
}

type Authorizer interface {
	HasAccess(ctx context.Context, objectID, action string) (bool, error)
}

type Manager interface {
	InternalOauth2ClientHolder
	GetOauth2ClientRegistrar(ctx context.Context) Oauth2ClientRegistrar
	GetAuthenticator(ctx context.Context) Authenticator
	GetAuthorizer(ctx context.Context) Authorizer
}
