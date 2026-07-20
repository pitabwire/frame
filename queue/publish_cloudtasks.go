package queue

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type createTaskRequest struct {
	Task createTask `json:"task"`
}

type createTask struct {
	Name         string            `json:"name,omitempty"`
	ScheduleTime string            `json:"scheduleTime,omitempty"`
	HTTPRequest  createHTTPRequest `json:"httpRequest"`
}

type createHTTPRequest struct {
	HTTPMethod string            `json:"httpMethod"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body"`
	OIDCToken  *createOIDCToken  `json:"oidcToken,omitempty"`
}

type createOIDCToken struct {
	ServiceAccountEmail string `json:"serviceAccountEmail"`
	Audience            string `json:"audience,omitempty"`
}

func (p *publisher) publishCloudTasks(ctx context.Context, body []byte, metadata map[string]string) error {
	if p.ctTarget == nil || p.httpClient == nil || p.tokenSource == nil {
		return errorsNotInit("cloudtasks")
	}

	tok, err := p.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("queue: cloudtasks token: %w", err)
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	for _, k := range []string{"traceparent", "tracestate", "baggage", "lang"} {
		if v := metadata[k]; v != "" {
			headers[k] = v
		}
	}

	task := createTask{
		HTTPRequest: createHTTPRequest{
			HTTPMethod: http.MethodPost,
			URL:        p.ctTarget.TargetURL,
			Headers:    headers,
			Body:       base64.StdEncoding.EncodeToString(body),
		},
	}
	if p.ctTarget.OIDCSA != "" {
		oidc := &createOIDCToken{ServiceAccountEmail: p.ctTarget.OIDCSA}
		if p.ctTarget.OIDCAudience != "" {
			oidc.Audience = p.ctTarget.OIDCAudience
		}
		task.HTTPRequest.OIDCToken = oidc
	}
	if p.ctTarget.TaskID != "" {
		task.Name = p.ctTarget.ResourceName + "/tasks/" + p.ctTarget.TaskID
	}
	if p.ctTarget.ScheduleDelay > 0 {
		task.ScheduleTime = time.Now().UTC().Add(p.ctTarget.ScheduleDelay).Format(time.RFC3339)
	}

	payload, err := json.Marshal(createTaskRequest{Task: task})
	if err != nil {
		return fmt.Errorf("queue: cloudtasks marshal: %w", err)
	}

	apiURL := p.ctTarget.CreateTaskAPIURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("queue: cloudtasks request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("queue: cloudtasks publish: %w", err)
	}
	defer drainAndClose(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("queue: cloudtasks publish: unexpected status %d", resp.StatusCode)
	}
	return nil
}
