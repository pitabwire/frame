package frame

import (
	"context"
	"gocloud.dev/server"
	"google.golang.org/grpc"
	"net/http"
	"testing"
	"time"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grpcServer := grpc.NewServer()

	srvOptions := &server.Options{}

	srv := NewService("Testing", GrpcServer(grpcServer, srvOptions))

	go func() {
		_ = srv.Run(ctx, ":")
	}()

	time.Sleep(time.Second*2)

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

	err = srv.Run(ctx, ":")
	if err != nil {
		t.Errorf("Could not run Server : %v", err)
	}
}
