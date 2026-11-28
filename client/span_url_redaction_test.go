package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pitabwire/frame/v3/client"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Not parallel: swaps the global tracer provider used by otelhttp.
func TestHTTPClientSpans_RedactQueryCredentialsInURLFull(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = tp.Shutdown(context.Background())
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	httpClient := client.NewHTTPClient(t.Context(), client.WithHTTPNoAuth())
	target := srv.URL + "/files?access_token=span-at-secret&X-Amz-Signature=span-sig-secret&sig=span-sig2&page=2"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	require.NoError(t, err)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var urlFull string
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			if attr.Key == "url.full" {
				urlFull = attr.Value.AsString()
			}
		}
	}
	require.NotEmpty(t, urlFull, "client span must carry url.full")
	require.Contains(t, urlFull, "page=2")
	for _, secret := range []string{"span-at-secret", "span-sig-secret", "span-sig2"} {
		require.NotContains(t, urlFull, secret)
	}
}
