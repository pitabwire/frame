package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// AuthHasAccess binary check to confirm if subject can perform action specified
func AuthHasAccess(ctx context.Context, action string, subject string) (bool, error) {

	authClaims := ClaimsFromContext(ctx)
	service := FromContext(ctx)

	config, ok := service.Config().(DefaultConfiguration)
	if !ok {
		return false, errors.New("could not cast setting to authorization config")
	}

	if authClaims == nil {
		return false, errors.New("only authenticated requsts should be used to check authorization")
	}

	payload := map[string]interface{}{
		"namespace":  authClaims.TenantID,
		"object":     authClaims.PartitionID,
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := service.InvokeRestService(ctx, http.MethodPost,
		config.AuthorizationServiceReadURI, payload, nil)
	if err != nil {
		return false, err
	}

	if status > 299 || status < 200 {
		return false, fmt.Errorf(" invalid response status %d had message %s", status, string(result))
	}

	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	if err != nil {
		return false, err
	}

	if val, ok := response["allowed"]; ok && val.(bool) {
		return true, nil
	}
	return false, nil
}
