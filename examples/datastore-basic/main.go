package main

import (
	"log"
	"net/http"
	"os"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("datastore ok"))
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

	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	if dbPool != nil {
		db := dbPool.DB(ctx, false)
		if db != nil {
			_ = db.Exec("select 1").Error
		}
	}

	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
