package httptor_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pitabwire/frame/v3/security/interceptors/httptor"
	"github.com/stretchr/testify/require"
)

func TestLoggingMiddleware_BodyLoggingMasksCredentials(t *testing.T) {
	ctx, logs := captureLogs(t)

	handler := httptor.LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"resp-at-secret","token_type":"bearer"}`))
	}), true)

	body := `{"username":"bob","password":"req-pw-secret"}`
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.Contains(t, logs.String(), "bob", "bodies are still logged when enabled")
	require.NotContains(t, logs.String(), "req-pw-secret")
	require.NotContains(t, logs.String(), "resp-at-secret")
}

func TestLoggingMiddleware_OversizedBodyReachesHandlerIntact(t *testing.T) {
	ctx, _ := captureLogs(t)
	const size = 1024*1024 + 4096 // larger than the 1 MiB logging cap
	payload := bytes.Repeat([]byte("a"), size)

	var received int
	var readErr error
	handler := httptor.LoggingMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		readErr = err
		received = len(b)
	}), true)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/upload", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "text/plain")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.NoError(t, readErr)
	require.Equal(t, size, received, "body logging must not truncate what the handler reads")
}
