package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/config"
)

// handler is a normal SubscribeWorker — identical for mem:// and push://.
type handler struct{}

func (h handler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
	util.Log(ctx).
		WithField("metadata_keys", len(metadata)).
		WithField("ce_type", metadata["ce-type"]).
		Info(string(message))
	return nil
}

func main() {
	// Local demos: disable secure push-auth requirement so push:// works without bearer.
	cfg := &config.ConfigurationDefault{
		RunServiceSecurely: false,
		ServiceName:        "queue-push-example",
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		util.Log(r.Context()).Info("app root")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "queue-push ok — POST /_frame/queue/events")
	})

	ctx, svc := frame.NewService(
		frame.WithConfig(cfg),
		frame.WithName("queue-push"),
		frame.WithHTTPHandler(http.DefaultServeMux),
		// Pull path still works for in-process tests:
		frame.WithRegisterPublisher("events-mem", "mem://events-mem"),
		frame.WithRegisterSubscriber("events-mem", "mem://events-mem", handler{}),
		// Push path: Knative / Cloud Tasks / curl → POST /_frame/queue/events
		frame.WithRegisterSubscriber("events", "push://events", handler{}),
	)

	svc.AddPreStartMethod(func(ctx context.Context, s *frame.Service) {
		_ = s.QueueManager().Publish(ctx, "events-mem", map[string]any{"message": "hello from mem"})
	})

	util.Log(ctx).Info("listening — try: curl -X POST localhost:8080/_frame/queue/events -d '{\"hi\":1}'")
	if err := svc.Run(ctx, ":8080"); err != nil {
		util.Log(ctx).WithError(err).Fatal("service stopped")
	}
}
