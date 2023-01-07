package frame

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpchello "google.golang.org/grpc/examples/helloworld/helloworld"
	"google.golang.org/grpc/test/bufconn"
	"log"
	"net"
	"strings"
	"testing"
	"time"
)

type grpcServer struct {
	grpchello.UnimplementedGreeterServer
}

func (s *grpcServer) SayHello(ctx context.Context, in *grpchello.HelloRequest) (
	*grpchello.HelloReply, error) {

	return &grpchello.HelloReply{Message: "Hello " + in.Name + " from frame"}, nil
}

func startGRPCServer() (*grpc.Server, *bufconn.Listener) {
	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)
	srv := grpc.NewServer()
	grpchello.RegisterGreeterServer(srv, &grpcServer{})

	go func() {
		if err := srv.Serve(listener); err != nil {
			log.Fatalf("failed to start grpc server: %+v", err)
		}
	}()
	return srv, listener
}

func getBufDialer(listener *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, url string) (net.Conn, error) {
		return listener.Dial()
	}
}

func TestRawGrpcServer(t *testing.T) {

	srv, listener := startGRPCServer()
	// it is here to properly stop the server
	defer func() { time.Sleep(10 * time.Millisecond) }()
	defer srv.Stop()

	err := clientInvokeGrpc(listener)
	if err != nil {
		t.Fatalf("failed to dial: %+v", err)
	}
}

func TestServiceGrpcServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)
	gsrv := grpc.NewServer()
	grpchello.RegisterGreeterServer(gsrv, &grpcServer{})

	var defConf ConfigurationDefault
	err := ConfigProcess("", &defConf)
	if err != nil {
		t.Errorf("Could not process test configurations %v", err)
		return
	}
	srv := NewService("Testing Service Grpc", GrpcServer(gsrv), ServerListener(listener), Config(&defConf))

	// it is here to properly stop the server
	defer func() { time.Sleep(100 * time.Millisecond) }()
	defer srv.Stop(ctx)

	go func() {
		err := srv.Run(ctx, "")
		if err != nil {
			srv.L().WithError(err).Error(" failed to run server ")
		}
	}()

	err = clientInvokeGrpc(listener)
	if err != nil {
		t.Fatalf("failed to dial: %+v", err)
	}
}

func clientInvokeGrpc(listener *bufconn.Listener) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(getBufDialer(listener)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	cli := grpchello.NewGreeterClient(conn)

	req := grpchello.HelloRequest{
		Name: "Testing Roma",
	}

	resp, err := cli.SayHello(ctx, &req)
	if err != nil {
		return err
	}

	if !strings.Contains(resp.Message, "frame") {
		return errors.New("The response message should contain the word frame ")
	}
	return conn.Close()
}

func TestService_Run(t *testing.T) {
	ctx := context.Background()

	srv := &Service{
		logger: logrus.New(),
	}

	go func() {
		err := srv.Run(ctx, ":")

		if err != nil {
			t.Errorf("Service can not be started successfully without a server")
		}
	}()

	time.Sleep(500 * time.Millisecond)
	srv.Stop(ctx)

	srv = NewService("Testing", NoopDriver())

	if err := srv.Run(ctx, ":"); err != nil {
		t.Errorf("Could not run Server : %+v", err)
	}

	srv.Stop(ctx)
}
