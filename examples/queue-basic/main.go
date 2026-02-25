package main

import (
	"context"
	"log"
	"net/http"

	"github.com/pitabwire/frame"
)

type handler struct{}

func (h handler) Handle(_ context.Context, _ map[string]string, message []byte) error {
	log.Printf("received message: %s", string(message))
	return nil
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("queue ok"))
	})

	ctx, svc := frame.NewService(
		frame.WithName("queue-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
		frame.WithRegisterPublisher("events", "mem://events"),
		frame.WithRegisterSubscriber("events", "mem://events", handler{}),
	)

	// Publish a test message once startup begins.
	svc.AddPreStartMethod(func(ctx context.Context, s *frame.Service) {
		_ = s.QueueManager().Publish(ctx, "events", map[string]any{"message": "hello"})
	})

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
