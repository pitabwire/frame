package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// AuthHasAccess binary check to confirm if subject can perform action specified
func AuthHasAccess(ctx context.Context, action string, subject string) (error, bool) {

	authClaims := ClaimsFromContext(ctx)
	service := FromContext(ctx)

	authorizationUrl := service.Config().(Configuration).AuthorizationServiceReadURI

	if authClaims == nil {
		return errors.New("only authenticated requsts should be used to check authorization"), false
	}

	payload := map[string]interface{}{
		"namespace":  authClaims.TenantID,
		"object":     authClaims.PartitionID,
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := service.InvokeRestService(ctx, http.MethodPost, authorizationUrl, payload, nil)
	if err != nil {
		return err, false
	}

	if status > 299 || status < 200 {
		return fmt.Errorf(" invalid response status %d had message %s", status, string(result)), false
	}

	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	if err != nil {
		return err, false
	}

	if val, ok := response["allowed"]; ok && val.(bool) {
		return nil, true
	}
	return nil, false
}
