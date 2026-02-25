package main

import (
	"fmt"
	"net/http"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		util.Log(r.Context()).Info("request received")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "hello from frame")
	})

	ctx, svc := frame.NewService(
		frame.WithName("http-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		util.Log(ctx).WithError(err).Fatal("service stopped")
	}
}
