package frame_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/pitabwire/frame"
	grpcping2 "github.com/pitabwire/frame/frametests/grpcping"
)

type grpcServer struct {
	grpcping2.UnimplementedFramePingServer
}

func (s *grpcServer) SayPing(_ context.Context, in *grpcping2.HelloRequest) (
	*grpcping2.HelloResponse, error) {
	return &grpcping2.HelloResponse{Message: "Hello " + in.GetName() + " from frame"}, nil
}

func startGRPCServer(_ *testing.T) (*grpc.Server, *bufconn.Listener) {
	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)
	srv := grpc.NewServer()
	grpcping2.RegisterFramePingServer(srv, &grpcServer{})

	go func() {
		if err := srv.Serve(listener); err != nil {
			_ = err
		}
	}()
	return srv, listener
}

func getBufDialer(listener *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(_ context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
}

func TestRawGrpcServer(t *testing.T) {
	srv, listener := startGRPCServer(t)
	// it is here to properly stop the server
	defer func() { time.Sleep(10 * time.Millisecond) }()
	defer srv.Stop()

	transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
	ctx, cancel, conn, err := getBufferedClConn(listener, transportCred)
	if err != nil {
		_ = err
		return
	}
	defer cancel()
	err = clientInvokeGrpc(ctx, conn)
	if err != nil {
		_ = err
	}
}

func TestServiceGrpcHealthServer(t *testing.T) {
	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)
	gsrv := grpc.NewServer()
	grpcping2.RegisterFramePingServer(gsrv, &grpcServer{})

	defConf, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
	if err != nil {
		_ = err
		return
	}
	defConf.ServerPort = ":40489"
	ctx, srv := frame.NewService(
		"Testing Service Grpc",
		frame.WithGRPCServer(gsrv),
		frame.WithGRPCServerListener(listener),
		frame.WithConfig(&defConf),
	)

	// Start the service in a goroutine.
	go func(ctx context.Context, srv *frame.Service) {
		runErr := srv.Run(ctx, "") // Changed srv.Start to srv.Run, empty port for bufconn
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, http.ErrServerClosed) {
			t.Errorf("TestServiceGrpcHealthServer: srv.Run failed: %v", runErr)
		}
	}(ctx, srv)
	defer func() {
		srv.Stop(ctx) // Ensure service is stopped
	}()

	// Add a delay to allow the server goroutine to start and listen.
	time.Sleep(100 * time.Millisecond)
	// Create a client connection using the bufconn listener.
	// The context returned by getBufferedClConn is for the client connection's lifecycle.
	transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
	ctx, cancel, conn, err := getBufferedClConn(listener, transportCred)
	if err != nil {
		t.Fatalf("TestServiceGrpcHealthServer: getBufferedClConn failed: %v", err)
		return
	}
	defer cancel()
	defer conn.Close()

	// Diagnostic delay
	time.Sleep(100 * time.Millisecond)

	// Invoke the gRPC health check.
	err = clientInvokeGrpcHealth(ctx, conn)
	if err != nil {
		t.Fatalf("TestServiceGrpcHealthServer: clientInvokeGrpcHealth failed: %v", err)
	}

	// Stop the service and check for errors.
	srv.Stop(ctx) // srv.Stop does not return an error
}

func TestServiceGrpcServer(t *testing.T) {
	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)
	gsrv := grpc.NewServer()
	grpcping2.RegisterFramePingServer(gsrv, &grpcServer{})

	defConf, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
	if err != nil {
		_ = err
		return
	}
	ctx, srv := frame.NewService(
		"Testing Service Grpc",
		frame.WithGRPCServer(gsrv),
		frame.WithGRPCServerListener(listener),
		frame.WithConfig(&defConf),
	)

	go func() {
		err = srv.Run(ctx, "")
		if err != nil {
			t.Logf(" failed to run server : %+v", err)
		}
	}()

	transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
	ctx, cancel, conn, err := getBufferedClConn(listener, transportCred)
	if err != nil {
		t.Errorf("could not get buffered connection : %+v", err)
	}
	defer cancel()
	err = clientInvokeGrpc(ctx, conn)
	if err != nil {
		t.Errorf("there was an error invoking client : %+v", err)
	}

	time.Sleep(2 * time.Second)
	srv.Stop(ctx)
}

func TestServiceGrpcTLSServer(_ *testing.T) {
	bufferSize := 10 * 1024 * 1024
	priListener := bufconn.Listen(bufferSize)

	defConf, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
	if err != nil {
		_ = err
		return
	}

	defConf.SetTLSCertAndKeyPath(
		"tests_runner/server-cert.pem",
		"tests_runner/server-key.pem",
	)

	ctx, srv := frame.NewService(
		"Testing Service Grpc",
		frame.WithConfig(&defConf),
		frame.WithServerListener(priListener),
	)

	gsrv := grpc.NewServer()
	grpcping2.RegisterFramePingServer(gsrv, &grpcServer{})

	srv.Init(ctx, frame.WithGRPCServer(gsrv), frame.WithGRPCPort(":50053"))

	go func() {
		err = srv.Run(ctx, "")
		if err != nil {
			srv.Log(ctx).WithError(err).Error(" failed to run server ")
		}
	}()
	time.Sleep(5 * time.Second)

	cert, err := os.ReadFile("tests_runner/ca-cert.pem")
	if err != nil {
		_ = err
		return
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		_ = err
		return
	}

	tlsConfig := &tls.Config{RootCAs: certPool}
	transportCred := grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))

	ctx, cancel, conn, err := getNetworkClConn("localhost:50053", transportCred)
	if err != nil {
		_ = err
		return
	}
	defer cancel()
	err = clientInvokeGrpc(ctx, conn)
	if err != nil {
		_ = err
	}

	time.Sleep(2 * time.Second)
	srv.Stop(ctx)
}

func getBufferedClConn(listener *bufconn.Listener, opts ...grpc.DialOption) (
	context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	opts = append(opts, grpc.WithContextDialer(getBufDialer(listener)))
	conn, err := grpc.NewClient("passthrough://bufnet", opts...)

	return ctx, cancel, conn, err
}

func getNetworkClConn(address string, opts ...grpc.DialOption) (
	context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	conn, err := grpc.NewClient(address, opts...)

	return ctx, cancel, conn, err
}

func clientInvokeGrpc(ctx context.Context, conn *grpc.ClientConn) error {
	cli := grpcping2.NewFramePingClient(conn)

	req := grpcping2.HelloRequest{
		Name: "Testing Roma",
	}

	resp, err := cli.SayPing(ctx, &req)
	if err != nil {
		return err
	}

	if !strings.Contains(resp.GetMessage(), "frame") {
		return errors.New("The response message should contain the word frame ")
	}
	return conn.Close()
}

func clientInvokeGrpcHealth(ctx context.Context, conn *grpc.ClientConn) error {
	cli := grpc_health_v1.NewHealthClient(conn)

	req := grpc_health_v1.HealthCheckRequest{
		Service: "Testing",
	}

	resp, err := cli.Check(ctx, &req)
	if err != nil {
		return err
	}

	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return errors.New("The response status should be all good ")
	}
	return conn.Close()
}

func TestService_Run(_ *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	ctx2, srv2 := frame.NewService("Testing", frame.WithServerListener(listener))

	go func() {
		if err := srv2.Run(ctx2, ":"); err != nil {
			if !errors.Is(err, context.Canceled) {
				_ = err
			}
		}
	}()

	time.Sleep(1 * time.Second)
	srv2.Stop(ctx2)
	time.Sleep(1 * time.Second)
}
