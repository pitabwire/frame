package protocol

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GCPPubSub decodes Google Cloud Pub/Sub push envelopes:
//
//	{
//	  "message": {
//	    "data": "<base64>",
//	    "attributes": { ... },
//	    "messageId": "...",
//	    "publishTime": "..."
//	  },
//	  "subscription": "projects/.../subscriptions/..."
//	}
//
// See https://cloud.google.com/pubsub/docs/push#receive_push
type GCPPubSub struct{}

func (GCPPubSub) Name() string { return ProtocolGCPPubSub }

func (GCPPubSub) Match(r *http.Request) bool {
	// Match only has headers. Pub/Sub push sets User-Agent to APIs-Google
	// (see https://cloud.google.com/pubsub/docs/push). Decode validates envelope.
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	if strings.Contains(ua, "apis-google") || strings.Contains(ua, "google-cloud-pubsub") {
		return true
	}
	if r.Header.Get("X-Goog-Pubsub-Subscription-Name") != "" {
		return true
	}
	return false
}

type gcpPushEnvelope struct {
	Message struct {
		Data        string            `json:"data"`
		Attributes  map[string]string `json:"attributes"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

func (GCPPubSub) Decode(r *http.Request) (*Inbound, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", errDecodeSentinel, err)
	}
	var env gcpPushEnvelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: not a gcp pubsub push envelope: %w", errDecodeSentinel, err)
	}
	if env.Message.MessageID == "" && env.Message.Data == "" && env.Subscription == "" {
		return nil, fmt.Errorf("%w: empty gcp pubsub push envelope", errDecodeSentinel)
	}

	var body []byte
	if env.Message.Data != "" {
		body, err = base64.StdEncoding.DecodeString(env.Message.Data)
		if err != nil {
			// Pub/Sub may use raw URL encoding in some paths; try RawStd.
			body, err = base64.URLEncoding.DecodeString(env.Message.Data)
			if err != nil {
				return nil, fmt.Errorf("%w: decode message.data: %w", errDecodeSentinel, err)
			}
		}
	}

	md := make(map[string]string)
	for k, v := range env.Message.Attributes {
		lk := strings.ToLower(k)
		if IsClaimKey(lk) {
			continue
		}
		md[lk] = v
		// Frame event header may arrive as attribute as-is or lowercased.
		if k == metaFrameEvent || lk == strings.ToLower(metaFrameEvent) {
			md[metaFrameEvent] = v
		}
	}
	if env.Message.MessageID != "" {
		md["pubsub.message_id"] = env.Message.MessageID
	}
	if env.Message.PublishTime != "" {
		md["pubsub.publish_time"] = env.Message.PublishTime
	}
	if env.Subscription != "" {
		md["pubsub.subscription"] = env.Subscription
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		md[metaContentType] = ct
	}
	return &Inbound{Body: body, Metadata: md}, nil
}
