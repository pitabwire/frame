package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		util.Log(r.Context()).Info("request received")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "datastore ok")
	})

	if os.Getenv("DATABASE_URL") == "" {
		util.Log(context.Background()).Warn("DATABASE_URL is not set; skipping datastore initialization")
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
		util.Log(ctx).WithError(err).Fatal("service stopped")
	}
}
