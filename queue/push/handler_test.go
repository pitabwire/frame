package push_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/queue/push"
	"github.com/pitabwire/frame/v2/workerpool"
	"github.com/stretchr/testify/require"
)

type captureHandler struct {
	mu       sync.Mutex
	metadata map[string]string
	body     []byte
	err      error
}

func (h *captureHandler) Handle(_ context.Context, metadata map[string]string, message []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metadata = metadata
	h.body = append([]byte(nil), message...)
	return h.err
}

func testWorkPool(t *testing.T) workerpool.Manager {
	t.Helper()
	cfg := &config.ConfigurationDefault{
		WorkerPoolCount:    4,
		WorkerPoolCapacity: 32,
	}
	mgr, err := workerpool.NewManager(t.Context(), cfg, func(context.Context, error) {})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	return mgr
}

func TestPushHandlerPOSTSuccessAndMethod405(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wp := testWorkPool(t)
	qm := queue.NewQueueManager(ctx, wp)
	lookup, ok := qm.(queue.PushLookup)
	require.True(t, ok)

	h := &captureHandler{}
	require.NoError(t, qm.AddSubscriber(ctx, "orders", "push://orders", h))
	require.NoError(t, qm.Init(ctx))

	mux := http.NewServeMux()
	appHits := 0
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		appHits++
		w.WriteHeader(http.StatusOK)
	})
	ph := push.NewHandler(lookup, push.Config{
		Auth:           push.NoneAuth{},
		HandlerTimeout: 5 * time.Second,
	})
	ph.Register(mux)

	// POST success
	req := httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader([]byte(`{"ok":true}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Roles", "admin") // must be stripped
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, 0, appHits)
	h.mu.Lock()
	require.JSONEq(t, `{"ok":true}`, string(h.body))
	require.NotContains(t, h.metadata, "roles")
	h.mu.Unlock()

	// GET → 405 (not app /)
	reqGet := httptest.NewRequest(http.MethodGet, "/_frame/queue/orders", nil)
	rrGet := httptest.NewRecorder()
	mux.ServeHTTP(rrGet, reqGet)
	require.Equal(t, http.StatusMethodNotAllowed, rrGet.Code)
	require.Equal(t, 0, appHits)
	require.Equal(t, http.MethodPost, rrGet.Header().Get("Allow"))

	// unknown ref → 404
	req404 := httptest.NewRequest(http.MethodPost, "/_frame/queue/missing", bytes.NewReader([]byte(`{}`)))
	rr404 := httptest.NewRecorder()
	mux.ServeHTTP(rr404, req404)
	require.Equal(t, http.StatusNotFound, rr404.Code)

	// app root still works
	reqApp := httptest.NewRequest(http.MethodGet, "/", nil)
	rrApp := httptest.NewRecorder()
	mux.ServeHTTP(rrApp, reqApp)
	require.Equal(t, http.StatusOK, rrApp.Code)
	require.Equal(t, 1, appHits)
}

func TestPushHandlerBearerAuth(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wp := testWorkPool(t)
	qm := queue.NewQueueManager(ctx, wp)
	lookup := qm.(queue.PushLookup)
	require.NoError(t, qm.AddSubscriber(ctx, "orders", "push://orders", &captureHandler{}))
	require.NoError(t, qm.Init(ctx))

	ph := push.NewHandler(lookup, push.Config{
		Auth: push.BearerAuth{Token: "secret"},
	})
	mux := http.NewServeMux()
	ph.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader([]byte(`{}`)))
	req2.Header.Set("Authorization", "Bearer secret")
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
}

func TestPushHandlerCloudEventsBinary(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wp := testWorkPool(t)
	qm := queue.NewQueueManager(ctx, wp)
	lookup := qm.(queue.PushLookup)
	h := &captureHandler{}
	require.NoError(t, qm.AddSubscriber(ctx, "orders", "push://orders", h))
	require.NoError(t, qm.Init(ctx))

	ph := push.NewHandler(lookup, push.Config{Auth: push.NoneAuth{}})
	mux := http.NewServeMux()
	ph.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader([]byte(`{"id":1}`)))
	req.Header.Set("Ce-Specversion", "1.0")
	req.Header.Set("Ce-Id", "abc")
	req.Header.Set("Ce-Type", "com.example.order")
	req.Header.Set("Ce-Source", "//test")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	h.mu.Lock()
	defer h.mu.Unlock()
	require.Equal(t, "com.example.order", h.metadata["ce-type"])
	require.Equal(t, "abc", h.metadata["ce-id"])
}

func TestPushHandlerNotRetryable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wp := testWorkPool(t)
	qm := queue.NewQueueManager(ctx, wp)
	lookup := qm.(queue.PushLookup)
	h := &captureHandler{err: queue.ErrNotRetryable}
	require.NoError(t, qm.AddSubscriber(ctx, "orders", "push://orders", h))
	require.NoError(t, qm.Init(ctx))

	ph := push.NewHandler(lookup, push.Config{Auth: push.NoneAuth{}})
	mux := http.NewServeMux()
	ph.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/_frame/queue/orders", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}
