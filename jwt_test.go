package frame

import (
	"context"
	"testing"
)

func TestService_RegisterForJwtWithParams(t *testing.T) {

	t.Skip("Only run this test manually by uncommenting line")

	ctx := context.Background()

	oauthServiceURL := "http://localhost:4447"
	clientName := "Testing CLI"
	clientSecret := "topS3cret"

	srv := NewService("Test Srv", Config(&ConfigurationDefault{
		Oauth2ServiceAdminURI: oauthServiceURL,
	}))

	response, err := srv.RegisterForJwtWithParams(
		ctx, oauthServiceURL, clientName, clientSecret,
		"", []string{}, map[string]string{})
	if err != nil {
		t.Errorf("couldn't register for jwt %s", err)
		return
	}

	srv.L().WithField("client id", response).Info("successfully registered for Jwt")

	err = srv.UnRegisterForJwt(ctx, oauthServiceURL, response)
	if err != nil {
		t.Errorf("couldn't un register for jwt %s", err)
		return
	}
}
