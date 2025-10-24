package permissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/pitabwire/frame/client"
	config2 "github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

type ketoAuthorizer struct {
	cfg    config2.ConfigurationAuthorization
	client client.Manager
}

func NewKetoAuthorizer(cfg config2.ConfigurationAuthorization, client client.Manager) security.Authorizer {
	return &ketoAuthorizer{
		cfg:    cfg,
		client: client,
	}
}

// HasAccess binary check to confirm if subject can perform action specified.
func (a *ketoAuthorizer) HasAccess(ctx context.Context, objectID, action string) (bool, error) {
	authClaims := security.ClaimsFromContext(ctx)

	if a.cfg == nil {
		return false, errors.New("could not get authorization config")
	}

	if authClaims == nil {
		return false, errors.New("only authenticated requests should be used to check authorization")
	}

	payload := map[string]any{
		"namespace":  authClaims.GetPartitionID(),
		"object":     objectID,
		"relation":   action,
		"subject_id": authClaims.GetProfileID(),
	}

	status, result, err := a.client.Invoke(ctx, http.MethodPost,
		a.cfg.GetAuthorizationServiceReadURI(), payload, nil)
	if err != nil {
		return false, err
	}

	if status > 299 || status < 200 {
		return false, fmt.Errorf(" invalid response status %d had message %s", status, string(result))
	}

	var response map[string]any
	err = json.Unmarshal(result, &response)
	if err != nil {
		return false, err
	}

	if val, allowedExists := response["allowed"]; allowedExists {
		if boolVal, typeOK := val.(bool); typeOK && boolVal {
			return true, nil
		}
	}
	return false, nil
}
