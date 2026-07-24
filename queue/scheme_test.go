package queue_test

import (
	"testing"

	"github.com/pitabwire/frame/v2/queue"
	"github.com/stretchr/testify/require"
)

func TestClassifySubscriberURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		mode queue.DeliveryMode
		ok   bool
	}{
		{"mem://events", queue.DeliveryModePull, true},
		{"nats://localhost/s", queue.DeliveryModePull, true},
		{"gcppubsub://projects/p/subscriptions/s", queue.DeliveryModePull, true},
		{"gcppubsub://myproject/mysub", queue.DeliveryModePull, true},
		{"push://orders", queue.DeliveryModePush, true},
		{"https://svc/_frame/queue/orders", queue.DeliveryModePush, true},
		{"http://localhost:8080/x", queue.DeliveryModePush, true},
		{"httpfoo://x", 0, false},
		{"", 0, false},
		{"://no", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			mode, _, err := queue.ClassifySubscriberURL(tc.url)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.mode, mode)
		})
	}
}

func TestClassifyPublisherURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		kind queue.PublishKind
		ok   bool
	}{
		{"mem://events", queue.PublishKindGoCloud, true},
		{"nats://localhost/s", queue.PublishKindGoCloud, true},
		{"gcppubsub://projects/p/topics/t", queue.PublishKindGoCloud, true},
		{"gcppubsub://myproject/mytopic", queue.PublishKindGoCloud, true},
		{"ce+https://broker/path?type=t", queue.PublishKindCloudEventsHTTP, true},
		{"ce+http://localhost/x?type=t", queue.PublishKindCloudEventsHTTP, true},
		{"cloudtasks:///projects/p/locations/l/queues/q?url=https://x/", queue.PublishKindCloudTasks, true},
		{"https://x", 0, false},
		{"push://x", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			kind, err := queue.ClassifyPublisherURL(tc.url)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.kind, kind)
		})
	}
}

func TestCloudEventsRequestURL(t *testing.T) {
	t.Parallel()
	reqURL, q, err := queue.CloudEventsRequestURL(
		"ce+https://broker.example/path?type=com.example.t&source=//svc",
	)
	require.NoError(t, err)
	require.Equal(t, "https://broker.example/path", reqURL)
	require.Equal(t, "com.example.t", q.Get("type"))
	require.Equal(t, "//svc", q.Get("source"))

	_, _, err = queue.CloudEventsRequestURL("https://x")
	require.Error(t, err)
}

func TestParseCloudTasksURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		url     string
		ok      bool
		project string
		depr    bool
	}{
		{
			name:    "canonical",
			url:     "cloudtasks:///projects/p/locations/l/queues/q?url=https://x/_frame/queue/o",
			ok:      true,
			project: "p",
		},
		{
			name:    "host-join deprecated",
			url:     "cloudtasks://projects/p/locations/l/queues/q?url=https://x/",
			ok:      true,
			project: "p",
			depr:    true,
		},
		{
			name:    "query form",
			url:     "cloudtasks://queue?project=p&location=l&queue=q&url=https://x/",
			ok:      true,
			project: "p",
		},
		{
			name: "empty project",
			url:  "cloudtasks:///projects//locations/l/queues/q?url=https://x/",
			ok:   false,
		},
		{
			name: "missing url",
			url:  "cloudtasks:///projects/p/locations/l/queues/q",
			ok:   false,
		},
		{
			name: "bad path",
			url:  "cloudtasks:///foo?url=https://x/",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := queue.ParseCloudTasksURL(tc.url)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.project, got.Project)
			require.Equal(t, "l", got.Location)
			require.Equal(t, "q", got.Queue)
			require.Equal(t, "projects/p/locations/l/queues/q", got.ResourceName)
			require.NotEmpty(t, got.TargetURL)
			require.Equal(t, tc.depr, got.DeprecatedHostForm)
			require.Contains(t, got.CreateTaskAPIURL(), "/v2/projects/p/locations/l/queues/q/tasks")
		})
	}
}
