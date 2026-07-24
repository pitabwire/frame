package protocol_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pitabwire/frame/v2/queue/protocol"
)

func TestGCPPubSubDecode(t *testing.T) {
	payload := []byte(`{"hello":"world"}`)
	env := map[string]any{
		"message": map[string]any{
			"data": base64.StdEncoding.EncodeToString(payload),
			"attributes": map[string]string{
				"frame._internal.event.header": "message.to.test",
				"traceparent":                  "00-abc-def-01",
			},
			"messageId":   "msg-1",
			"publishTime": "2026-07-24T00:00:00Z",
		},
		"subscription": "projects/p/subscriptions/s",
	}
	body, _ := json.Marshal(env)
	req := httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "APIs-Google; (+https://developers.google.com/webmasters/APIs-Google.html)")

	c := protocol.GCPPubSub{}
	if !c.Match(req) {
		t.Fatal("expected Match true for APIs-Google UA")
	}
	// body was consumed by... no Match doesn't read body
	req = httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "APIs-Google; (+https://developers.google.com/webmasters/APIs-Google.html)")

	in, err := c.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	if string(in.Body) != string(payload) {
		t.Fatalf("body=%q want %q", in.Body, payload)
	}
	if in.Metadata["frame._internal.event.header"] != "message.to.test" {
		t.Fatalf("event header missing: %#v", in.Metadata)
	}
	if in.Metadata["pubsub.message_id"] != "msg-1" {
		t.Fatalf("message id missing: %#v", in.Metadata)
	}
}

func TestSelectCodecGCPPubSub(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("User-Agent", "APIs-Google")
	c := protocol.SelectCodec("auto", req, protocol.DefaultCodecs())
	if c.Name() != protocol.ProtocolGCPPubSub {
		t.Fatalf("got %s want %s", c.Name(), protocol.ProtocolGCPPubSub)
	}
}
