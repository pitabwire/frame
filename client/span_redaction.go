package client

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/pitabwire/frame/v3/internal/redact"
)

// urlFullKey is the OpenTelemetry semantic-convention key otelhttp uses for
// the full request URL on client spans.
const urlFullKey = attribute.Key("url.full")

// spanURLRedactingTransport sits directly inside otelhttp.Transport. otelhttp
// strips userinfo from url.full but keeps the query string, which routinely
// carries credentials (access_token, presigned-URL signatures, ...). By the
// time this transport runs otelhttp has started the span and recorded
// url.full, so overwriting the attribute here replaces it with the redacted
// form before the span is exported.
type spanURLRedactingTransport struct {
	inner http.RoundTripper
}

func (t spanURLRedactingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if span := trace.SpanFromContext(req.Context()); span.IsRecording() && req.URL != nil {
		u := *req.URL
		u.User = nil
		span.SetAttributes(urlFullKey.String(redact.URL(u.String())))
	}
	return t.inner.RoundTrip(req)
}
