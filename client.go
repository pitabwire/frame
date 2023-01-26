package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
)

// InvokeRestService convenience method to call a http endpoint and utilize the raw results
func (s *Service) InvokeRestService(ctx context.Context,
	method string, endpointURL string, payload map[string]interface{},
	headers map[string][]string) (int, []byte, error) {

	if headers == nil {
		headers = map[string][]string{
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		}
	}

	var body io.Reader
	if payload != nil {
		postBody, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}

		body = bytes.NewBuffer(postBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, body)
	if err != nil {
		return 0, nil, err
	}

	req.Header = headers

	reqDump, _ := httputil.DumpRequestOut(req, true)

	s.L().WithField("request", string(reqDump)).Info("request out")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	s.L().WithField("response", string(respDump)).Info("response in")

	defer resp.Body.Close()

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err

}
