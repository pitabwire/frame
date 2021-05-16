package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)


func AuthorizationControlListHasAccess(ctx context.Context, action string, subject string) (error, bool) {

	authorizationUrl := fmt.Sprintf("%s%s", GetEnv("KETO_AUTHORIZATION_READ_URL", ""), "/check")

	authClaims := ClaimsFromContext(ctx)

	if authClaims == nil {
		return errors.New("only authenticated requsts should be used to check authorization"), false
	}

	payload := map[string]interface{}{
		"namespace": authClaims.TenantID,
		"object":    authClaims.PartitionID,
		"relation":  action,
		"subject":   subject,
	}

	err, result := invokeACLService(ctx, http.MethodPost, authorizationUrl, payload)

	if err != nil {
		return err, false
	}

	if val, ok := result["allowed"]; ok && val.(bool) {
		return nil, true
	}
	return nil, false
}


func invokeACLService(ctx context.Context, method string, authorizationUrl string, payload map[string]interface{}) (error, map[string]interface{}) {

	headers := map[string][]string{
		"Content-Type": {"application/json"},
		"Accept":       {"application/json"},
	}

	service := FromContext(ctx)

	postBody, err := json.Marshal(payload)
	if err != nil {
		return err, nil
	}

	req, err := http.NewRequestWithContext(ctx, method, authorizationUrl, bytes.NewBuffer(postBody))
	//Handle Error
	if err != nil {
		return err, nil
	}
	req.Header = headers

	resp, err := service.client.Do(req)
	if err != nil {
		return err, nil
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err, nil
	}

	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err, nil
	}

	return nil, response
}
