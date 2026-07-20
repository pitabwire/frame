package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	ceContentTypeStructured = "application/cloudevents+json"
	defaultJSONContentType  = "application/json"
)

// CloudEvents decodes CloudEvents 1.0 binary and structured content modes.
type CloudEvents struct{}

func (CloudEvents) Name() string { return ProtocolCloudEvents }

func (CloudEvents) Match(r *http.Request) bool {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, ceContentTypeStructured) {
		return true
	}
	if r.Header.Get("Ce-Id") != "" || r.Header.Get("Ce-Type") != "" || r.Header.Get("Ce-Specversion") != "" {
		return true
	}
	for k := range r.Header {
		if strings.HasPrefix(strings.ToLower(k), "ce-") {
			return true
		}
	}
	return false
}

func (c CloudEvents) Decode(r *http.Request) (*Inbound, error) {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, ceContentTypeStructured) {
		return c.decodeStructured(r)
	}
	return c.decodeBinary(r)
}

func (CloudEvents) decodeBinary(r *http.Request) (*Inbound, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", errDecodeSentinel, err)
	}
	md := mapBinaryHeaders(r)
	if dct := md["ce-datacontenttype"]; dct != "" && md[metaContentType] == "" {
		md[metaContentType] = dct
	}
	return &Inbound{Body: body, Metadata: md}, nil
}

func mapBinaryHeaders(r *http.Request) map[string]string {
	md := make(map[string]string)
	for k, vals := range r.Header {
		if len(vals) == 0 {
			continue
		}
		lk := strings.ToLower(k)
		switch {
		case strings.HasPrefix(lk, "ce-"):
			md[lk] = vals[0]
			if lk == metaCEFrameEvent {
				md[metaFrameEvent] = vals[0]
			}
		case lk == metaContentType, lk == metaTraceParent, lk == metaTraceState, lk == metaBaggage:
			md[lk] = vals[0]
		}
	}
	return md
}

func (CloudEvents) decodeStructured(r *http.Request) (*Inbound, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", errDecodeSentinel, err)
	}
	envelope, err := parseCEEnvelope(raw)
	if err != nil {
		return nil, err
	}
	md := mapStructuredAttrs(envelope)
	copyOTelHeaders(md, r)
	body, err := extractStructuredData(envelope)
	if err != nil {
		return nil, err
	}
	if ct := md["ce-datacontenttype"]; ct != "" {
		md[metaContentType] = ct
	} else if md[metaContentType] == "" {
		md[metaContentType] = defaultJSONContentType
	}
	return &Inbound{Body: body, Metadata: md}, nil
}

func parseCEEnvelope(raw []byte) (map[string]any, error) {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("%w: structured cloudevents: %w", errDecodeSentinel, err)
	}
	return envelope, nil
}

func mapStructuredAttrs(envelope map[string]any) map[string]string {
	md := make(map[string]string)
	for k, v := range envelope {
		if k == "data" || k == "data_base64" {
			continue
		}
		lk := strings.ToLower(k)
		md["ce-"+lk] = fmt.Sprint(v)
		if lk == "frameevent" {
			md[metaFrameEvent] = fmt.Sprint(v)
		}
	}
	return md
}

func copyOTelHeaders(md map[string]string, r *http.Request) {
	for _, h := range []string{metaTraceParent, metaTraceState, metaBaggage} {
		if v := r.Header.Get(h); v != "" {
			md[h] = v
		}
	}
}

func extractStructuredData(envelope map[string]any) ([]byte, error) {
	if b64, hasB64 := envelope["data_base64"].(string); hasB64 && b64 != "" {
		return []byte(b64), nil
	}
	data, hasData := envelope["data"]
	if !hasData {
		return nil, nil
	}
	if s, ok := data.(string); ok {
		return []byte(s), nil
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal data: %w", errDecodeSentinel, err)
	}
	return body, nil
}

// errDecodeSentinel mirrors queue.ErrDecode message for errors.Is with wrapped errors.
var errDecodeSentinel = errors.New("queue: decode failed")
