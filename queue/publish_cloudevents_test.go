package queue_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/workerpool"
	"github.com/stretchr/testify/require"
)

func TestCloudEventsPublisherBinaryEgress(t *testing.T) {
	t.Parallel()

	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	ctx := t.Context()
	cfg := &config.ConfigurationDefault{WorkerPoolCount: 2, WorkerPoolCapacity: 8}
	wp, err := workerpool.NewManager(ctx, cfg, func(context.Context, error) {})
	require.NoError(t, err)
	t.Cleanup(func() { _ = wp.Shutdown(context.Background()) })

	qm := queue.NewQueueManager(ctx, wp,
		queue.WithHTTPClient(srv.Client()),
		queue.WithServiceName("orders-svc"),
	)
	// httptest URL is http://127.0.0.1:port — prefix with ce+ for Frame scheme.
	pubURL := "ce+" + srv.URL + "/sink?type=com.example.order.created&source=//orders-svc"

	require.NoError(t, qm.AddPublisher(ctx, "orders", pubURL))
	require.NoError(t, qm.Init(ctx))

	payload := map[string]any{"id": "1"}
	require.NoError(t, qm.Publish(ctx, "orders", payload))

	require.NotNil(t, got)
	require.Equal(t, http.MethodPost, got.Method)
	require.Equal(t, "1.0", got.Header.Get("Ce-Specversion"))
	require.NotEmpty(t, got.Header.Get("Ce-Id"))
	require.Equal(t, "//orders-svc", got.Header.Get("Ce-Source"))
	require.Equal(t, "com.example.order.created", got.Header.Get("Ce-Type"))
	require.Equal(t, "application/json", got.Header.Get("Content-Type"))
	// scheme stripped: request went to http not ce+http
	require.Equal(t, "/sink", got.URL.Path)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Equal(t, "1", decoded["id"])
}
