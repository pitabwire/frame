package frame_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pitabwire/frame"
	"net/http"
	"testing"
)

func authorizationControlListWrite(ctx context.Context, action string, subject string) error {
	authClaims := frame.ClaimsFromContext(ctx)
	service := frame.FromContext(ctx)
	config, ok := service.Config().(frame.ConfigurationAuthorization)
	if !ok {
		return errors.New("could not cast setting to authorization config")
	}

	if authClaims == nil {
		return errors.New("only authenticated requsts should be used to check authorization")
	}

	payload := map[string]interface{}{
		"namespace":  authClaims.TenantID,
		"object":     authClaims.PartitionID,
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := service.InvokeRestService(ctx,
		http.MethodPut, config.GetAuthorizationServiceWriteURI(), payload, nil)

	if err != nil {
		return err
	}

	if status > 299 || status < 200 {
		return fmt.Errorf(" invalid response status %d had message %s", status, string(result))
	}

	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	if err != nil {
		return err
	}

	return nil
}

func TestAuthorizationControlListWrite(t *testing.T) {

	ctx := context.Background()
	srv := frame.NewService("Test Srv", frame.Config(frame.ConfigurationDefault{
		AuthorizationServiceWriteURI: "http://localhost:4467/relation-tuples",
	}))
	ctx = frame.ToContext(ctx, srv)

	authClaim := frame.AuthenticationClaims{
		TenantID:    "default",
		PartitionID: "partition",
		ProfileID:   "profile",
		AccessID:    "access",
	}
	ctx = authClaim.ClaimsToContext(ctx)

	err := authorizationControlListWrite(ctx, "read", "tested")
	if err != nil {
		t.Errorf("Authorization write was not possible see %+v", err)
		return
	}

}

func TestAuthHasAccess(t *testing.T) {
	ctx := context.Background()
	srv := frame.NewService("Test Srv", frame.Config(
		frame.ConfigurationDefault{
			AuthorizationServiceReadURI:  "http://localhost:4466/check",
			AuthorizationServiceWriteURI: "http://localhost:4467/relation-tuples",
		}))
	ctx = frame.ToContext(ctx, srv)

	authClaim := frame.AuthenticationClaims{
		TenantID:    "default",
		PartitionID: "partition",
		ProfileID:   "profile",
		AccessID:    "access",
	}
	ctx = authClaim.ClaimsToContext(ctx)

	err := authorizationControlListWrite(ctx, "read", "reader")
	if err != nil {
		t.Errorf("Authorization write was not possible see %+v", err)
		return
	}

	access, err := frame.AuthHasAccess(ctx, "read", "reader")
	if err != nil {
		t.Errorf("Authorization check was not possible see %+v", err)
	} else if !access {
		t.Errorf("Authorization check was forbidden")
		return
	}

	access, err = frame.AuthHasAccess(ctx, "read", "read-master")
	if err == nil || access {
		t.Errorf("Authorization check was not forbidden yet shouldn't exist")
		return
	}

}
