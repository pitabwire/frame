package queue_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/pitabwire/frame/v3/config"
	"github.com/pitabwire/frame/v3/queue"
	"github.com/pitabwire/frame/v3/workerpool"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
)

type noopWorker struct{}

func (noopWorker) Handle(context.Context, map[string]string, []byte) error { return nil }

// A queue URL that cannot be opened must not leak its credentials into the
// error returned to the service or into the failure log lines.
func TestQueueManagerInit_NeverLeaksURLCredentials(t *testing.T) {
	const password = "nats-pw-0f9e8d"

	cases := []struct {
		name     string
		register func(ctx context.Context, qm queue.Manager) error
	}{
		{
			name: "publisher with unreachable server",
			register: func(ctx context.Context, qm queue.Manager) error {
				return qm.AddPublisher(ctx, "pub", "nats://svc:"+password+"@127.0.0.1:1/orders?jetstream=true")
			},
		},
		{
			name: "subscriber with unreachable server",
			register: func(ctx context.Context, qm queue.Manager) error {
				return qm.AddSubscriber(ctx, "sub",
					"nats://svc:"+password+"@127.0.0.1:1/orders?jetstream=true&consumer=c1", noopWorker{})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := &bytes.Buffer{}
			logger := util.NewLogger(t.Context(),
				util.WithLogOutput(logs),
				util.WithLogLevel(slog.LevelDebug),
				util.WithLogNoColor(true),
				util.WithLogHandlerExclusive(),
			)
			ctx, cancel := context.WithTimeout(util.ContextWithLogger(t.Context(), logger), 30*time.Second)
			defer cancel()

			cfg := &config.ConfigurationDefault{WorkerPoolCount: 2, WorkerPoolCapacity: 8}
			wp, err := workerpool.NewManager(ctx, cfg, func(context.Context, error) {})
			require.NoError(t, err)
			t.Cleanup(func() { _ = wp.Shutdown(context.Background()) })

			qm := queue.NewQueueManager(ctx, wp)
			require.NoError(t, tc.register(ctx, qm))

			initErr := qm.Init(ctx)

			require.Error(t, initErr, "opening an unreachable queue must fail")
			require.NotContains(t, initErr.Error(), password, "returned error must not carry the URL password")
			require.NotEmpty(t, logs.String(), "the failure must be logged")
			require.NotContains(t, logs.String(), password, "logs must not carry the URL password")
		})
	}
}

func TestClassifyURL_ErrorsNeverLeakCredentials(t *testing.T) {
	t.Parallel()
	const password = "classify-pw-77"
	raw := "nats://svc:" + password + "@host:4222/\x7f"

	_, _, subErr := queue.ClassifySubscriberURL(raw)
	require.Error(t, subErr)
	require.NotContains(t, subErr.Error(), password)

	_, pubErr := queue.ClassifyPublisherURL(raw)
	require.Error(t, pubErr)
	require.NotContains(t, pubErr.Error(), password)

	_, ctErr := queue.ParseCloudTasksURL("cloudtasks://svc:" + password + "@x/\x7f")
	require.Error(t, ctErr)
	require.NotContains(t, ctErr.Error(), password)
}
