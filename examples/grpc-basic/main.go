package main

import (
	"fmt"
	"log"
	"net/http"

	"google.golang.org/grpc"

	"github.com/pitabwire/frame"
)

func main() {
	grpcServer := grpc.NewServer()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "http ok — %s %s", r.Method, r.URL.Path)
	})

	ctx, svc := frame.NewService(
		frame.WithName("grpc-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
		frame.WithGRPCServer(grpcServer),
		frame.WithGRPCPort(":50051"),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
