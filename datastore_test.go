package frame_test

import (
	"context"
	"github.com/pitabwire/frame"
	"testing"
)

const testDatastoreConnection = "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable"

func TestService_Datastore(t *testing.T) {
	ctx := context.Background()
	mainDb := frame.Datastore(ctx, testDatastoreConnection, false)

	srv := frame.NewService("Test Srv", mainDb)

	if srv.Name() != "Test Srv" {
		t.Errorf("s")
	}

	w := srv.DB(ctx, false)
	if w == nil {
		t.Errorf("No default service could be instantiated")
		return
	}

	r := srv.DB(ctx, true)
	if r == nil {
		t.Errorf("Could not get read db instantiated")
		return
	}

	wd, _ := w.DB()
	rd, _ := r.DB()
	if wd != rd {
		t.Errorf("Read and write db services should not be different ")
	}

	srv.Stop()
}

func TestService_DatastoreRead(t *testing.T) {
	ctx := context.Background()
	mainDb := frame.Datastore(ctx, testDatastoreConnection, false)
	readDb := frame.Datastore(ctx, testDatastoreConnection, true)

	srv := frame.NewService("Test Srv", mainDb, readDb)

	w := srv.DB(ctx, false)
	r := srv.DB(ctx, true)
	if w == nil || r == nil {
		t.Errorf("Read and write services setup but one couldn't be found")
		return
	}

	rd, _ := r.DB()
	if wd, _ := w.DB(); wd == rd {
		t.Errorf("Read and write db services are same but we set different")
	}
}

func TestService_DatastoreNotSet(t *testing.T) {
	ctx := context.Background()

	srv := frame.NewService("Test Srv")

	w := srv.DB(ctx, false)
	if w != nil {
		t.Errorf("When no connection is set no db is expected")
	}

}
