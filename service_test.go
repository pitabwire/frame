package frame

import (
	"context"
	"gocloud.dev/server"
	"testing"
)

func TestDefaultService(t *testing.T) {

	srv := NewService( "Test Srv")
	if srv == nil {
		t.Errorf("No default service could be instaniated")
		return
	}

	if srv.name != "Test Srv" {
		t.Errorf("s")
	}
}

func TestService(t *testing.T) {

	srvOptions := &server.Options{

	}
	opt := HttpServer(srvOptions)


	srv := NewService( "Test", opt)
	if srv == nil {
		t.Errorf("No default service could be instaniated")
	}
}


func TestFromContext(t *testing.T) {

	ctx := context.Background()

	srv := NewService( "Test Srv")

	nullSrv := FromContext(ctx)
	if nullSrv != nil {
		t.Errorf("Service was found in context yet it was not set")
	}

	ctx = ToContext(ctx, srv)

	valueSrv := FromContext(ctx)
	if valueSrv == nil {
		t.Errorf("No default service was found in context")
	}

}

func TestService_AddCleanupMethod(t *testing.T) {

	srv := NewService( "Test Srv")

	a := 30

	srv.AddCleanupMethod(func() {
		a += 1
	})

	srv.AddCleanupMethod(func() {
		a += 1
	})


	if a != 30{
		t.Errorf("Clean up method is running prematurely")
	}


	srv.Stop()

	if a != 32{
		t.Errorf("Clean up method is not running at shutdown")
	}
}

type testHC struct {
}

func (h *testHC) CheckHealth() error {
	return nil
}

func TestService_AddHealthCheck(t *testing.T) {

	srv := NewService( "Test Srv")


	healthChecker := new(testHC)

	if srv.healthCheckers != nil {
		t.Errorf("Health checkers are not supposed to be added by default")
	}

	srv.AddHealthCheck(healthChecker)

	if len(srv.healthCheckers) == 0 {
		t.Errorf("Health checkers are not being added to list")
	}
}

