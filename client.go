package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// InvokeRestService convenience method to call a http endpoint and utilize the raw results.
func (s *Service) InvokeRestService(ctx context.Context,
	method string, endpointURL string, payload map[string]any,
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

	s.Log(ctx).WithField("request", string(reqDump)).Debug("request out")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respDump, _ := httputil.DumpResponse(resp, true)
	s.Log(ctx).WithField("response", string(respDump)).Debug("response in")

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}

// InvokeRestServiceURLEncoded sends an HTTP request to the specified endpoint with a URL-encoded payload.
func (s *Service) InvokeRestServiceURLEncoded(ctx context.Context,
	method string, endpointURL string, payload url.Values,
	headers map[string]string) (int, []byte, error) {
	if headers == nil {
		headers = map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}
	}

	logger := s.Log(ctx).WithField("method", method).WithField("endpoint", endpointURL).WithField("header", headers)

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return 0, nil, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	reqDump, _ := httputil.DumpRequestOut(req, true)
	logger.WithField("request", string(reqDump)).Info("request out")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respDump, _ := httputil.DumpResponse(resp, true)
	s.Log(ctx).WithField("response", string(respDump)).Info("response in")

	response, err := io.ReadAll(resp.Body)

	return resp.StatusCode, response, err
}
