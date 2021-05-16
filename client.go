package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func InvokeRestService(ctx context.Context, method string, authorizationUrl string, payload map[string]interface{}, headers map[string][]string) ([]byte, error) {

	if headers == nil {

		headers = map[string][]string{
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		}
	}

	service := FromContext(ctx)

	postBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, authorizationUrl, bytes.NewBuffer(postBody))
	//Handle Error
	if err != nil {
		return nil, err
	}
	req.Header = headers

	resp, err := service.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)

}

