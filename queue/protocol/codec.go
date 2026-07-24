package protocol

import (
	"net/http"
	"strings"
)

// Inbound is a decoded push message ready for processDelivery.
type Inbound struct {
	Body     []byte
	Metadata map[string]string
}

// Codec decodes an HTTP push request into body + metadata.
type Codec interface {
	Name() string
	Match(r *http.Request) bool
	Decode(r *http.Request) (*Inbound, error)
}

// Protocol preference from subscriber URL query protocol=.
const (
	ProtocolAuto        = "auto"
	ProtocolRaw         = "raw"
	ProtocolCloudEvents = "cloudevents"
	ProtocolCloudTasks  = "cloudtasks"
	ProtocolGCPPubSub   = "gcppubsub"
)

// DefaultCodecs returns the ordered codec list for protocol=auto.
func DefaultCodecs() []Codec {
	return []Codec{
		CloudTasks{},
		CloudEvents{},
		GCPPubSub{},
		Raw{},
	}
}

// SelectCodec picks a codec for the request based on protocol preference.
func SelectCodec(protocol string, r *http.Request, codecs []Codec) Codec {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = ProtocolAuto
	}

	switch protocol {
	case ProtocolRaw:
		return Raw{}
	case ProtocolCloudEvents:
		return CloudEvents{}
	case ProtocolCloudTasks:
		return CloudTasks{}
	case ProtocolGCPPubSub:
		return GCPPubSub{}
	default:
		// auto
		for _, c := range codecs {
			if c.Match(r) {
				return c
			}
		}
		return Raw{}
	}
}
