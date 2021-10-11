package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

// InvokeRestService convenience method to call a http endpoint and utilize the raw results
func (s *Service) InvokeRestService(ctx context.Context, method string, endpointUrl string, payload map[string]interface{}, headers map[string][]string) (int, []byte, error) {

	if headers == nil {
		headers = map[string][]string{
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		}
	}

	postBody, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointUrl, bytes.NewBuffer(postBody))
	if err != nil {
		return 0, nil, err
	}
	req.Header = headers

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	defer resp.Body.Close()

	response, err := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, response, err

}
