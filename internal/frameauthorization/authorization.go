package frameauthorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/pitabwire/frame/internal/common"
)

// AuthHasAccess binary check to confirm if subject can perform action specified.
func AuthHasAccess(ctx context.Context, action string, subject string) (bool, error) {
	authClaims := common.ClaimsFromContext(ctx)
	service := common.Svc

	config, ok := service.Config().(common.ConfigurationAuthorization)
	if !ok {
		return false, errors.New("could not cast setting to authorization config")
	}

	if authClaims == nil {
		return false, errors.New("only authenticated requsts should be used to check authorization")
	}

	payload := map[string]any{
		"namespace":  authClaims.GetTenantID(),
		"object":     authClaims.GetPartitionID(),
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := service.InvokeRestService(ctx, http.MethodPost,
		config.GetAuthorizationServiceReadURI(), payload, nil)
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
