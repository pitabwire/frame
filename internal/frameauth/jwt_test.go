package frameauth_test

import (
	"testing"

	"github.com/pitabwire/frame"
)

func TestService_RegisterForJwtWithParams(t *testing.T) {
	t.Skip("Only run this test manually by uncommenting line")

	oauthServiceURL := "http://localhost:4447"
	clientName := "Testing CLI"
	clientID := "test-cli-dev"
	clientSecret := "topS3cret"

	ctx, srv := frame.NewService("Test Srv", frame.WithConfig(&frame.ConfigurationDefault{
		Oauth2ServiceAdminURI: oauthServiceURL,
	}))

	response, err := srv.RegisterForJwtWithParams(
		ctx, oauthServiceURL, clientName, clientID, clientSecret,
		"", []string{}, map[string]string{})
	if err != nil {
		t.Errorf("couldn't register for jwt %s", err)
		return
	}

	srv.SetJwtClient(response)

	srv.Log(ctx).WithField("client id", response).Info("successfully registered for Jwt")

	err = srv.UnRegisterForJwt(ctx, oauthServiceURL, srv.JwtClientID())
	if err != nil {
		t.Errorf("couldn't un register for jwt %s", err)
		return
	}
}
