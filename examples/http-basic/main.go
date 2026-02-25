package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/pitabwire/frame"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "hello from frame")
	})

	ctx, svc := frame.NewService(
		frame.WithName("http-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
