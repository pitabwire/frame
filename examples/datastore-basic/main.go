package main

import (
    "context"
    "log"
    "net/http"
    "os"

    "github.com/pitabwire/frame"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("datastore ok"))
    })

    if os.Getenv("DATABASE_URL") == "" {
        log.Println("DATABASE_URL is not set; skipping datastore initialization")
        return
    }

    ctx, svc := frame.NewService(
        frame.WithName("datastore-basic"),
        frame.WithHTTPHandler(http.DefaultServeMux),
        frame.WithDatastore(),
    )

    db := svc.DatastoreManager().DB(ctx, false)
    if db != nil {
        _ = db.Exec("select 1").Error
    }

    if err := svc.Run(ctx, ":8080"); err != nil {
        log.Fatal(err)
    }
}
