package main

import (
	"log"
	"net/http"

	"github.com/pitabwire/frame"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello from frame"))
	})

	ctx, svc := frame.NewService(
		frame.WithName("http-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
