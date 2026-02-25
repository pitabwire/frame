package main

import (
	"fmt"
	"net/http"

	"github.com/pitabwire/util"
	"google.golang.org/grpc"

	"github.com/pitabwire/frame"
)

func main() {
	grpcServer := grpc.NewServer()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		util.Log(r.Context()).Info("request received")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "http ok")
	})

	ctx, svc := frame.NewService(
		frame.WithName("grpc-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
		frame.WithGRPCServer(grpcServer),
		frame.WithGRPCPort(":50051"),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		util.Log(ctx).WithError(err).Fatal("service stopped")
	}
}
