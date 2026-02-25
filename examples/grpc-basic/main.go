package main

import (
	"log"
	"net/http"

	"google.golang.org/grpc"

	"github.com/pitabwire/frame"
)

func main() {
	grpcServer := grpc.NewServer()

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("http ok"))
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
