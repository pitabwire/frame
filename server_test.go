package frame

import (
	"context"
	"errors"
	"gocloud.dev/server"
	"net/http"
	"testing"
)

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

	err := srv.Run(ctx, "", func(ctx context.Context, s interface{}) error {
		return nil
	})

	if err == nil{
		t.Errorf("Service can not be started successfully without a server")
	}


	srv = NewService("Testing")
	err = srv.Run(ctx, "", func(ctx context.Context, s interface{}) error {
		return errors.New("error in server setup")
	})

	if err == nil || err.Error() != "error in server setup"{
		t.Errorf("Server setup with failure is not being captured")
	}

	srvOptions := &server.Options{
		Driver: &testDriver{},
	}

	srv = NewService("Testing", HttpServer(srvOptions))

	err = srv.Run(ctx, ":40000", func(ctx context.Context, s interface{}) error {
		return nil
	})
	if err != nil{
		t.Errorf("Could not run Server : %v", err)
	}
}


