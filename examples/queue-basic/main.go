package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/pitabwire/frame"
)

type handler struct{}

func (h handler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
	log.Printf("[%v] received message: %s (metadata keys: %d)",
		ctx.Err(), string(message), len(metadata))
	return nil
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "queue ok")
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
