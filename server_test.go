package frame

import (
	"context"
	"gocloud.dev/server"
	"google.golang.org/grpc"
	"net/http"
	"testing"
)

type testDriver struct {
}

func (t *testDriver) ListenAndServe(addr string, h http.Handler) error {
	return nil
}

func (t *testDriver) Shutdown(ctx context.Context) error {
	return nil
}

func TestGrpcServer(t *testing.T) {

	ctx := context.Background()

	grpcServer := grpc.NewServer()

	srvOptions := &server.Options{
		Driver: &testDriver{},
	}

	srv := NewService("Testing", GrpcServer(grpcServer, srvOptions))

	go func() {
		err := srv.Run(ctx, ":40000")

		if err != nil {
			t.Errorf("Could not run Server : %v", err)
		}
	}()

	srv.Stop()

}

func TestService_Run(t *testing.T) {
	ctx := context.Background()
	srv := &Service{}

	err := srv.Run(ctx, "")

	if err == nil {
		t.Errorf("Service can not be started successfully without a server")
	}

	srvOptions := &server.Options{
		Driver: &testDriver{},
	}

	srv = NewService("Testing", HttpServer(srvOptions))

	err = srv.Run(ctx, ":40000")
	if err != nil {
		t.Errorf("Could not run Server : %v", err)
	}
}
