package main

import (
    "log"
    "net/http"

    "github.com/pitabwire/frame"
    "google.golang.org/grpc"
)

func main() {
    grpcServer := grpc.NewServer()

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("http ok"))
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
