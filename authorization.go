package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)


func AuthorizationControlListHasAccess(ctx context.Context, action string, subject string) (error, bool) {

	authorizationUrl := fmt.Sprintf("%s%s", GetEnv("KETO_AUTHORIZATION_READ_URL", ""), "/check")

	authClaims := ClaimsFromContext(ctx)
	service := FromContext(ctx)

	if authClaims == nil {
		return errors.New("only authenticated requsts should be used to check authorization"), false
	}

	payload := map[string]interface{}{
		"namespace": authClaims.TenantID,
		"object":    authClaims.PartitionID,
		"relation":  action,
		"subject":   subject,
	}

	result, err := service.InvokeRestService(ctx, http.MethodPost, authorizationUrl, payload, nil)
	if err != nil {
		return err, false
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


