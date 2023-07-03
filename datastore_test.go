package frame_test

import (
	"github.com/pitabwire/frame"
	"os"
	"testing"
)

func TestService_Datastore(t *testing.T) {

	testDBURL := frame.GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	mainDB := frame.DatastoreCon(testDBURL, false)

	ctx, srv := frame.NewService("Test Srv", mainDB, frame.NoopDriver())

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

	rd, _ := r.DB()
	if wd, _ := w.DB(); wd != rd {
		t.Errorf("Read and write db services should not be different ")
	}

	srv.Stop(ctx)

}

func TestService_DatastoreSet(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	var defConf frame.ConfigurationDefault
	err := frame.ConfigProcess("", &defConf)
	if err != nil {
		t.Errorf("Could not processFunc test configurations %v", err)
		return
	}
	ctx, srv := frame.NewService("Test Srv", frame.Config(&defConf))
	srv.Init(frame.Datastore(ctx))

	w := srv.DB(ctx, false)
	r := srv.DB(ctx, true)
	if w == nil || r == nil {
		t.Errorf("Read and write services setup but one couldn't be found")
		return
	}
}

func TestService_DatastoreRead(t *testing.T) {
	testDBURL := frame.GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	mainDB := frame.DatastoreCon(testDBURL, false)
	readDB := frame.DatastoreCon(testDBURL, true)

	ctx, srv := frame.NewService("Test Srv", mainDB, readDB)

	w := srv.DB(ctx, false)
	r := srv.DB(ctx, true)
	if w == nil || r == nil {
		t.Errorf("Read and write services setup but one couldn't be found")
		return
	}

	rd, _ := r.DB()
	wd, _ := w.DB()
	if wd == rd {
		t.Errorf("Read and write db services are same but we set different")
	}
}

func TestService_DatastoreNotSet(t *testing.T) {

	ctx, srv := frame.NewService("Test Srv")

	if w := srv.DB(ctx, false); w != nil {
		t.Errorf("When no connection is set no db is expected")
	}
}
