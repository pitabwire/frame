package main

import (
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/pitabwire/frame"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "hello from frame — %s %s", r.Method, html.EscapeString(r.URL.Path))
	})

	ctx, svc := frame.NewService(
		frame.WithName("http-basic"),
		frame.WithHTTPHandler(http.DefaultServeMux),
	)

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
