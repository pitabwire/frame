package queue_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/workerpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type staticTokenSource struct{ token string }

func (s staticTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.token, Expiry: time.Now().Add(time.Hour)}, nil
}

func TestCloudTasksPublisherCreateTaskBody(t *testing.T) {
	t.Parallel()

	var (
		method string
		path   string
		auth   string
		raw    []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"projects/p/locations/l/queues/q/tasks/1"}`))
	}))
	t.Cleanup(srv.Close)

	// Patch CreateTask host by using a custom HTTP client that rewrites?
	// Instead, we inject URL via transport that redirects cloudtasks.googleapis.com to srv.
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	ctx := t.Context()
	cfg := &config.ConfigurationDefault{WorkerPoolCount: 2, WorkerPoolCapacity: 8}
	wp, err := workerpool.NewManager(ctx, cfg, func(context.Context, error) {})
	require.NoError(t, err)
	t.Cleanup(func() { _ = wp.Shutdown(context.Background()) })

	targetURL := "https://orders.example.com/_frame/queue/orders"
	pubURL := "cloudtasks:///projects/p/locations/l/queues/q?url=" +
		"https%3A%2F%2Forders.example.com%2F_frame%2Fqueue%2Forders" +
		"&oidc_sa=tasks-invoker@my-proj.iam.gserviceaccount.com" +
		"&oidc_audience=" + targetURL +
		"&task_id=task-1"

	qm := queue.NewQueueManager(ctx, wp,
		queue.WithHTTPClient(client),
		queue.WithTokenSource(staticTokenSource{token: "adc-token"}),
	)
	require.NoError(t, qm.AddPublisher(ctx, "orders", pubURL))
	require.NoError(t, qm.Init(ctx))

	payload := []byte(`{"hello":"world"}`)
	require.NoError(t, qm.Publish(ctx, "orders", payload))

	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/v2/projects/p/locations/l/queues/q/tasks", path)
	require.Equal(t, "Bearer adc-token", auth)

	var env struct {
		Task struct {
			Name        string `json:"name"`
			HTTPRequest struct {
				HTTPMethod string `json:"httpMethod"`
				URL        string `json:"url"`
				Body       string `json:"body"`
				OIDCToken  *struct {
					ServiceAccountEmail string `json:"serviceAccountEmail"`
					Audience            string `json:"audience"`
				} `json:"oidcToken"`
			} `json:"httpRequest"`
		} `json:"task"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	require.Equal(t, "POST", env.Task.HTTPRequest.HTTPMethod)
	require.Equal(t, targetURL, env.Task.HTTPRequest.URL)
	decoded, err := base64.StdEncoding.DecodeString(env.Task.HTTPRequest.Body)
	require.NoError(t, err)
	// internal.Marshal of []byte may JSON-encode as base64 string or raw — accept either contains hello
	require.Contains(t, string(decoded), "hello")
	require.NotNil(t, env.Task.HTTPRequest.OIDCToken)
	require.Equal(
		t,
		"tasks-invoker@my-proj.iam.gserviceaccount.com",
		env.Task.HTTPRequest.OIDCToken.ServiceAccountEmail,
	)
	require.Equal(t, targetURL, env.Task.HTTPRequest.OIDCToken.Audience)
	require.Equal(t, "projects/p/locations/l/queues/q/tasks/task-1", env.Task.Name)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
