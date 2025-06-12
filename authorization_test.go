package frame_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/pitabwire/frame"
)

func authorizationControlListWrite(ctx context.Context, writeServerURL string, action string, subject string) error {
	authClaims := frame.ClaimsFromContext(ctx)
	service := frame.Svc(ctx)

	if authClaims == nil {
		return errors.New("only authenticated requsts should be used to check authorization")
	}

	payload := map[string]any{
		"namespace":  authClaims.GetTenantID(),
		"object":     authClaims.GetPartitionID(),
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := service.InvokeRestService(ctx,
		http.MethodPut, writeServerURL, payload, nil)

	if err != nil {
		return err
	}

	if status > 299 || status < 200 {
		return fmt.Errorf(" invalid response status %d had message %s", status, string(result))
	}

	var response map[string]any
	err = json.Unmarshal(result, &response)
	if err != nil {
		return err
	}

	return nil
}

func TestAuthorizationControlListWrite(t *testing.T) {
	authorizationServerURL := "http://localhost:4467/admin/relation-tuples"
	ctx, srv := frame.NewService("Test Srv", frame.WithConfig(&frame.ConfigurationDefault{
		AuthorizationServiceWriteURI: authorizationServerURL,
	}))
	ctx = frame.SvcToContext(ctx, srv)

	authClaim := frame.AuthenticationClaims{
		Ext: map[string]any{
			"partition_id": "partition",
			"tenant_id":    "default",
			"access_id":    "access",
		},
	}
	authClaim.Subject = "profile"
	ctx = authClaim.ClaimsToContext(ctx)

	err := authorizationControlListWrite(ctx, authorizationServerURL, "read", "tested")
	if err != nil {
		t.Errorf("Authorization write was not possible see %s", err)
		return
	}
}

func TestAuthHasAccess(t *testing.T) {
	authorizationServerURL := "http://localhost:4467/admin/relation-tuples"
	ctx, srv := frame.NewService("Test Srv", frame.WithConfig(
		&frame.ConfigurationDefault{
			AuthorizationServiceReadURI:  "http://localhost:4466/relation-tuples/check",
			AuthorizationServiceWriteURI: authorizationServerURL,
		}))
	ctx = frame.SvcToContext(ctx, srv)

	authClaim := frame.AuthenticationClaims{
		Ext: map[string]any{
			"partition_id": "partition",
			"tenant_id":    "default",
			"access_id":    "access",
		}}
	authClaim.Subject = "profile"
	ctx = authClaim.ClaimsToContext(ctx)

	err := authorizationControlListWrite(ctx, authorizationServerURL, "read", "reader")
	if err != nil {
		t.Errorf("Authorization write was not possible see %s", err)
		return
	}

	access, err := frame.AuthHasAccess(ctx, "read", "reader")
	if err != nil {
		t.Errorf("Authorization check was not possible see %s", err)
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
