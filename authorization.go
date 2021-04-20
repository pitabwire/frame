package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func AuthorizationControlListWrite(ctx context.Context, action string, subject string) error {

	authorizationUrl := fmt.Sprintf("%s%s", GetEnv("KETO_AUTHORIZATION_WRITE_URL", ""), "/relation-tuples")
	err, _ := invokeACLService(ctx, action, subject, http.MethodPut, authorizationUrl)

	if err != nil {
		return err
	}

	return nil
}

func AuthorizationControlListHasAccess(ctx context.Context, action string, subject string) (error, bool) {

	authorizationUrl := fmt.Sprintf("%s%s", GetEnv("KETO_AUTHORIZATION_READ_URL", ""), "/check")
	err, result := invokeACLService(ctx, action, subject, http.MethodPost, authorizationUrl)

	if err != nil {
		return err, false
	}

	if val, ok := result["allowed"]; ok && val.(bool) {
		return nil, true
	}
	return nil, false
}


func invokeACLService(ctx context.Context, action string, subject string, method string, authorizationUrl string) (error, map[string]interface{}) {

	headers := map[string][]string{
		"Content-Type": {"application/json"},
		"Accept":       {"application/json"},
	}

	service := FromContext(ctx)

	postBody, _ := json.Marshal(map[string]interface{}{
		"namespace": "default",
		"object":    service.name,
		"relation":  action,
		"subject":   subject,
	})



	req, err := http.NewRequestWithContext(ctx, method, authorizationUrl, bytes.NewBuffer(postBody))
	//Handle Error
	if err != nil {
		log.Printf(" problems at request : %v", err)
		return err, nil
	}
	req.Header = headers

	resp, err := service.client.Do(req)
	if err != nil {
		log.Printf(" problems after request : %v", err)
		return err, nil
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf(" problems at body close : %v", err)
		return err, nil
	}

	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Printf(" problems at decoding message : %s  %v", string(body), err)
		return err, nil
	}

	return nil, response
}
