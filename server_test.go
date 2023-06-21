package frame

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpchello "google.golang.org/grpc/examples/helloworld/helloworld"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	"log"
	"net"
	"os"
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
			log.Fatalf("failed to start grpc server: %s", err)
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

	transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
	ctx, cancel, conn, err := getBufferedClConn(listener, transportCred)
	if err != nil {
		t.Errorf("unable to open a connection %s", err)
		return
	}
	defer cancel()
	err = clientInvokeGrpc(ctx, conn)
	if err != nil {
		t.Fatalf("failed to dial: %s", err)
	}
}

func TestServiceGrpcHealthServer(t *testing.T) {

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
	defConf.ServerPort = ":40489"
	ctx, srv := NewService("Testing Service Grpc", GrpcServer(gsrv), GrpcServerListener(listener), Config(&defConf))

	go func(t *testing.T, srv *Service) {
		err = srv.Run(ctx, "")
		if err != nil {
			srv.L().WithError(err).Error(" failed to run server ")
		}
	}(t, srv)

	transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
	ctx, cancel, conn, err := getBufferedClConn(listener, transportCred)
	if err != nil {
		t.Errorf("unable to open a connection %s", err)
		return
	}
	defer cancel()
	err = clientInvokeGrpcHealth(ctx, conn)
	if err != nil {
		t.Fatalf("failed to dial: %s", err)
	}

	time.Sleep(2 * time.Second)
	srv.Stop(ctx)
}

func TestServiceGrpcServer(t *testing.T) {

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
	ctx, srv := NewService("Testing Service Grpc", GrpcServer(gsrv), GrpcServerListener(listener), Config(&defConf))

	go func() {
		err = srv.Run(ctx, "")
		if err != nil {
			srv.L().WithError(err).Error(" failed to run server ")
		}
	}()

	transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
	ctx, cancel, conn, err := getBufferedClConn(listener, transportCred)
	if err != nil {
		t.Errorf("unable to open a connection %s", err)
		return
	}
	defer cancel()
	err = clientInvokeGrpc(ctx, conn)
	if err != nil {
		t.Fatalf("failed to dial: %s", err)
	}

	time.Sleep(2 * time.Second)
	srv.Stop(ctx)
}

func TestServiceGrpcTLSServer(t *testing.T) {

	bufferSize := 10 * 1024 * 1024
	priListener := bufconn.Listen(bufferSize)

	var defConf ConfigurationDefault
	err := ConfigProcess("", &defConf)
	if err != nil {
		t.Errorf("Could not process test configurations %v", err)
		return
	}

	defConf.SetTLSCertAndKeyPath("tests_runner/server-cert.pem", "tests_runner/server-key.pem")

	ctx, srv := NewService("Testing Service Grpc", Config(&defConf), ServerListener(priListener))

	//tlsCreds, err := credentials.NewServerTLSFromFile(defConf.TLSCertPath(), defConf.TLSCertKeyPath())
	//if err != nil {
	//	t.Errorf("Could not utilize an empty config for tls %s", err)
	//	return
	//}

	gsrv := grpc.NewServer()
	grpchello.RegisterGreeterServer(gsrv, &grpcServer{})

	srv.Init(GrpcServer(gsrv), GrpcPort(":50053"))

	go func() {
		err = srv.Run(ctx, "")
		if err != nil {
			srv.L().WithError(err).Error(" failed to run server ")
		}
		srv.L().WithError(err).Error(" finished building TLS service")
	}()
	time.Sleep(5 * time.Second)

	cert, err := os.ReadFile("tests_runner/ca-cert.pem")
	if err != nil {
		t.Errorf("Could not read ca cert file %v", err)
		return
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		t.Errorf("unable to parse cert from %s", "tests_runner/ca-cert.pem")
		return
	}

	tlsConfig := &tls.Config{RootCAs: certPool}
	transportCred := grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))

	ctx, cancel, conn, err := getNetworkClConn("localhost:50053", transportCred)
	if err != nil {
		t.Errorf("unable to open a connection %s", err)
		return
	}
	defer cancel()
	err = clientInvokeGrpc(ctx, conn)
	if err != nil {
		t.Fatalf("failed to dial: %s", err)
	}

	time.Sleep(2 * time.Second)
	srv.Stop(ctx)
}

func getBufferedClConn(listener *bufconn.Listener, opts ...grpc.DialOption) (
	context.Context, context.CancelFunc, *grpc.ClientConn, error) {

	ctx, cancel := context.WithCancel(context.Background())

	opts = append(opts, grpc.WithContextDialer(getBufDialer(listener)))
	conn, err := grpc.DialContext(ctx, "", opts...)

	return ctx, cancel, conn, err

}

func getNetworkClConn(address string, opts ...grpc.DialOption) (
	context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	conn, err := grpc.DialContext(ctx, address, opts...)

	return ctx, cancel, conn, err
}

func clientInvokeGrpc(ctx context.Context, conn *grpc.ClientConn) error {

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

func TestService_Run(t *testing.T) {

	listener := bufconn.Listen(1024 * 1024)
	ctx2, srv2 := NewService("Testing", ServerListener(listener))

	go func() {
		if err := srv2.Run(ctx2, ":"); err != nil {
			if !errors.Is(context.Canceled, err) {
				t.Errorf("Could not run Server : %s", err)
			}
		}
	}()

	time.Sleep(1 * time.Second)
	srv2.Stop(ctx2)
	time.Sleep(1 * time.Second)
}
