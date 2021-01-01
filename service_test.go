package frame

import (
	"context"
	"errors"
	"gocloud.dev/server"
	"net/http"
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
	opt := Server(srvOptions)


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

type testDriver struct {
}

func (t *testDriver) ListenAndServe(addr string, h http.Handler) error{
	return nil
}

func (t *testDriver) Shutdown(ctx context.Context) error{
	return nil
}

func TestService_Run(t *testing.T) {
	ctx := context.Background()
	srv := &Service{}

	err := srv.Run(ctx, "", func(ctx context.Context, s *server.Server) error {
		return nil
	})

	if err == nil{
		t.Errorf("Service can not be started successfully without a server")
	}


	srv = NewService("Testing")
	err = srv.Run(ctx, "", func(ctx context.Context, s *server.Server) error {
		return errors.New("error in server setup")
	})

	if err == nil || err.Error() != "error in server setup"{
		t.Errorf("Server setup with failure is not being captured")
	}

	srvOptions := &server.Options{
		Driver: &testDriver{},
	}

	srv = NewService("Testing", Server(srvOptions))

	err = srv.Run(ctx, ":40000", func(ctx context.Context, s *server.Server) error {
		return nil
	})
	if err != nil{
		t.Errorf("Could not run Server : %v", err)
	}
}