package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/xid"
)

// frameEventHeader is the Frame events package metadata key.
// Mapped to CE extension "frameevent" on the wire (alphanumeric only).
const (
	frameEventHeader      = "frame._internal.event.header"
	ceFrameEventExtension = "frameevent"
)

type cloudEventsPublishConfig struct {
	requestURL      string
	eventType       string
	source          string
	dataContentType string
	subject         string
	timeout         time.Duration
}

func parseCloudEventsPublishConfig(rawURL, publisherRef, serviceName string) (*cloudEventsPublishConfig, error) {
	reqURL, q, err := CloudEventsRequestURL(rawURL)
	if err != nil {
		return nil, err
	}
	cfg := &cloudEventsPublishConfig{
		requestURL:      reqURL,
		eventType:       q.Get("type"),
		source:          q.Get("source"),
		dataContentType: q.Get("datacontenttype"),
		subject:         q.Get("subject"),
	}
	if cfg.dataContentType == "" {
		cfg.dataContentType = "application/json"
	}
	if cfg.source == "" {
		svc := serviceName
		if svc == "" {
			svc = "frame"
		}
		cfg.source = fmt.Sprintf("//%s/%s", svc, publisherRef)
	}
	if t := q.Get("timeout"); t != "" {
		if d, durErr := time.ParseDuration(t); durErr == nil {
			cfg.timeout = d
		}
	}
	// Prefer hard Init when type is present on URL; empty allowed here,
	// Publish will require type via metadata or error.
	return cfg, nil
}

func (p *publisher) publishCloudEvents(ctx context.Context, body []byte, metadata map[string]string) error {
	if p.ceConfig == nil || p.httpClient == nil {
		return errorsNotInit("cloudevents")
	}

	eventType := metadata["ce-type"]
	if eventType == "" {
		eventType = p.ceConfig.eventType
	}
	if eventType == "" {
		return errors.New("queue: cloudevents type is required")
	}

	source := metadata["ce-source"]
	if source == "" {
		source = p.ceConfig.source
	}

	id := metadata["ce-id"]
	if id == "" {
		id = xid.New().String()
	}

	contentType := metadata["content-type"]
	if contentType == "" {
		contentType = p.ceConfig.dataContentType
	}

	reqCtx := ctx
	var cancel context.CancelFunc
	if p.ceConfig.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, p.ceConfig.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.ceConfig.requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("queue: cloudevents request: %w", err)
	}

	// Required CE 1.0 binary-mode attributes
	req.Header.Set("Ce-Specversion", "1.0")
	req.Header.Set("Ce-Id", id)
	req.Header.Set("Ce-Source", source)
	req.Header.Set("Ce-Type", eventType)
	req.Header.Set("Content-Type", contentType)

	if subject := firstNonEmpty(metadata["ce-subject"], p.ceConfig.subject); subject != "" {
		req.Header.Set("Ce-Subject", subject)
	}
	if t := metadata["ce-time"]; t != "" {
		req.Header.Set("Ce-Time", t)
	} else {
		req.Header.Set("Ce-Time", time.Now().UTC().Format(time.RFC3339))
	}

	// Map Frame event header → CE extension frameevent
	if ev := metadata[frameEventHeader]; ev != "" {
		req.Header.Set("Ce-"+strings.ToUpper(ceFrameEventExtension[:1])+ceFrameEventExtension[1:], ev)
		// CE HTTP binary mode uses Ce-<Attribute> with attribute names lowercased on wire often as Ce-Frameevent
		req.Header.Set("Ce-Frameevent", ev)
	}

	// Propagate OTel + safe headers
	for _, k := range []string{"traceparent", "tracestate", "baggage"} {
		if v := metadata[k]; v != "" {
			req.Header.Set(k, v)
		}
	}
	if lang := metadata["lang"]; lang != "" {
		req.Header.Set("Lang", lang)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("queue: cloudevents publish: %w", err)
	}
	defer drainAndClose(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("queue: cloudevents publish: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func errorsNotInit(kind string) error {
	return fmt.Errorf("publisher is not initialized for %s", kind)
}
