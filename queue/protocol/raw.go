package protocol

import (
	"io"
	"net/http"
	"strings"
)

// Raw decodes a plain HTTP body with a minimal safe header allowlist.
type Raw struct{}

func (Raw) Name() string { return ProtocolRaw }

func (Raw) Match(_ *http.Request) bool {
	// Fallback only; auto order places Raw last.
	return true
}

func (Raw) Decode(r *http.Request) (*Inbound, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	md := make(map[string]string)
	for k, vals := range r.Header {
		if len(vals) == 0 {
			continue
		}
		lk := strings.ToLower(k)
		if IsClaimKey(lk) {
			continue
		}
		if !isAllowedRawHeader(lk) {
			continue
		}
		md[lk] = vals[0]
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		md[metaContentType] = ct
	}
	return &Inbound{Body: body, Metadata: md}, nil
}
