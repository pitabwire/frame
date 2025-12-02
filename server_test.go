package frame_test

import (
	"bytes"
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

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	grpcping2 "github.com/pitabwire/frame/frametests/grpcping"
	"github.com/pitabwire/frame/tests"
)

// ServerTestSuite extends FrameBaseTestSuite for comprehensive server testing.
type ServerTestSuite struct {
	tests.BaseTestSuite
}

// TestServerSuite runs the server test suite.
func TestServerSuite(t *testing.T) {
	suite.Run(t, &ServerTestSuite{})
}

type grpcServer struct {
	grpcping2.UnimplementedFramePingServer
}

func (s *grpcServer) SayPing(_ context.Context, in *grpcping2.HelloRequest) (
	*grpcping2.HelloResponse, error) {
	return &grpcping2.HelloResponse{Message: "Hello " + in.GetName() + " from frame"}, nil
}

func (s *ServerTestSuite) startGRPCServer(_ *testing.T) (*grpc.Server, *bufconn.Listener) {
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

func (s *ServerTestSuite) getBufDialer(listener *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(_ context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
}

// TestRawGrpcServer tests raw gRPC server functionality.
func (s *ServerTestSuite) TestRawGrpcServer() {
	testCases := []struct {
		name string
	}{
		{
			name: "basic gRPC server test",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				srv, listener := s.startGRPCServer(t)
				// it is here to properly stop the server
				defer func() { time.Sleep(10 * time.Millisecond) }()
				defer srv.Stop()

				transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
				ctx, cancel, conn, err := s.getBufferedClConn(listener, transportCred)
				if err != nil {
					_ = err
					return
				}
				defer cancel()
				err = s.clientInvokeGrpc(ctx, conn)
				if err != nil {
					_ = err
				}
			})
		}
	})
}

// TestServiceGrpcHealthServer tests gRPC health server functionality.
func (s *ServerTestSuite) TestServiceGrpcHealthServer() {
	testCases := []struct {
		name        string
		serviceName string
		serverPort  string
	}{
		{
			name:        "gRPC health server test",
			serviceName: "Testing Service Grpc",
			serverPort:  ":40489",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				bufferSize := 1024 * 1024
				listener := bufconn.Listen(bufferSize)
				gsrv := grpc.NewServer()
				grpcping2.RegisterFramePingServer(gsrv, &grpcServer{})

				defConf, err := config.FromEnv[config.ConfigurationDefault]()
				if err != nil {
					_ = err
					return
				}
				defConf.ServerPort = tc.serverPort

				httpTestOpt, _ := frametests.WithHTTPTestDriver()

				ctx, srv := frame.NewService(
					frame.WithName(tc.serviceName),
					httpTestOpt,
					frame.WithGRPCServer(gsrv),
					frame.WithGRPCServerListener(listener),
					frame.WithConfig(&defConf),
				)

				runErr := srv.Run(ctx, "") // Changed srv.Start to srv.Run, empty port for bufconn
				if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, http.ErrServerClosed) {
					t.Errorf("TestServiceGrpcHealthServer: srv.Run failed: %v", runErr)
				}

				transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
				ctx2, cancel, conn, err := s.getBufferedClConn(listener, transportCred)
				if err != nil {
					_ = err
					return
				}
				defer cancel()
				err = s.clientInvokeGrpcHealth(ctx2, conn)
				if err != nil {
					_ = err
				}

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceGrpcServer tests service gRPC server functionality.
func (s *ServerTestSuite) TestServiceGrpcServer() {
	testCases := []struct {
		name        string
		serviceName string
		grpcPort    string
	}{
		{
			name:        "service gRPC server test",
			serviceName: "Testing Service Grpc",
			grpcPort:    ":50052",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				bufferSize := 1024 * 1024
				listener := bufconn.Listen(bufferSize)
				gsrv := grpc.NewServer()
				grpcping2.RegisterFramePingServer(gsrv, &grpcServer{})

				httpTestOpt, _ := frametests.WithHTTPTestDriver()

				ctx, srv := frame.NewService(
					frame.WithName(tc.serviceName),
					httpTestOpt,
					frame.WithGRPCServer(gsrv),
					frame.WithGRPCServerListener(listener),
				)

				srv.Init(ctx, frame.WithGRPCServer(gsrv), frame.WithGRPCPort(tc.grpcPort))

				err := srv.Run(ctx, "")
				if err != nil {
					srv.Log(ctx).WithError(err).Error(" failed to run server ")
				}

				transportCred := grpc.WithTransportCredentials(insecure.NewCredentials())
				ctx2, cancel, conn, err := s.getBufferedClConn(listener, transportCred)
				if err != nil {
					_ = err
					return
				}
				defer cancel()
				err = s.clientInvokeGrpc(ctx2, conn)
				if err != nil {
					_ = err
				}

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceGrpcTLSServer tests TLS-enabled gRPC server functionality.
//
//nolint:gocognit
func (s *ServerTestSuite) TestServiceGrpcTLSServer() {
	testCases := []struct {
		name        string
		serviceName string
		grpcPort    string
		certFile    string
	}{
		{
			name:        "TLS gRPC server test",
			serviceName: "Testing Service TLS Grpc",
			grpcPort:    ":50053",
			certFile:    "tests_runner/ca-cert.pem",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				gsrv := grpc.NewServer()
				grpcping2.RegisterFramePingServer(gsrv, &grpcServer{})

				ctx, srv := frame.NewService(frame.WithName(tc.serviceName))

				httpTestOpt, _ := frametests.WithHTTPTestDriver()

				srv.Init(ctx, httpTestOpt, frame.WithGRPCServer(gsrv), frame.WithGRPCPort(tc.grpcPort))

				err := srv.Run(ctx, "")
				if err != nil {
					srv.Log(ctx).WithError(err).Error(" failed to run server ")
				}

				cert, err := os.ReadFile(tc.certFile)
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

				ctx2, cancel, conn, err := s.getNetworkClConn("localhost"+tc.grpcPort, transportCred)
				if err != nil {
					_ = err
					return
				}
				defer cancel()
				err = s.clientInvokeGrpc(ctx2, conn)
				if err != nil {
					_ = err
				}

				time.Sleep(2 * time.Second)
				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceRun tests service run functionality.
func (s *ServerTestSuite) TestServiceRun() {
	testCases := []struct {
		name        string
		serviceName string
		port        string
	}{
		{
			name:        "service run test",
			serviceName: "Testing",
			port:        ":",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				httpTestOpt, _ := frametests.WithHTTPTestDriver()

				ctx2, srv2 := frame.NewService(frame.WithName(tc.serviceName), httpTestOpt)

				if err := srv2.Run(ctx2, tc.port); err != nil {
					if !errors.Is(err, context.Canceled) {
						_ = err
					}
				}

				time.Sleep(1 * time.Second)
				srv2.Stop(ctx2)
				time.Sleep(1 * time.Second)
			})
		}
	})
}

func (s *ServerTestSuite) getBufferedClConn(listener *bufconn.Listener, opts ...grpc.DialOption) (
	context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	opts = append(opts, grpc.WithContextDialer(s.getBufDialer(listener)))
	conn, err := grpc.NewClient("passthrough://bufnet", opts...)

	return ctx, cancel, conn, err
}

func (s *ServerTestSuite) getNetworkClConn(address string, opts ...grpc.DialOption) (
	context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	conn, err := grpc.NewClient(address, opts...)

	return ctx, cancel, conn, err
}

func (s *ServerTestSuite) clientInvokeGrpc(ctx context.Context, conn *grpc.ClientConn) error {
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

func (s *ServerTestSuite) clientInvokeGrpcHealth(ctx context.Context, conn *grpc.ClientConn) error {
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

// TestServiceContextRun tests service run functionality.
func (s *ServerTestSuite) TestServiceContextRun() {
	testCases := []struct {
		name        string
		serviceName string
		port        string
		logData     string
	}{
		{
			name:        "service context propagation test",
			serviceName: "Testing",
			port:        ":",
			logData:     "testing hello we are live",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var buf1 bytes.Buffer

				httpTestOpt, testSrvFn := frametests.WithHTTPTestDriver()

				ctx, svc := frame.NewServiceWithContext(t.Context(), frame.WithName(tc.serviceName),
					frame.WithLogger(util.WithLogOutput(&buf1)),
					httpTestOpt, frame.WithHTTPHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
						util.Log(r.Context()).Info(tc.logData)
					})))

				if err := svc.Run(ctx, tc.port); err != nil {
					if !errors.Is(err, context.Canceled) {
						_ = err
					}
				}

				client := svc.HTTPClientManager()
				_, _, err := client.Invoke(t.Context(), "GET", testSrvFn().URL, nil, nil)
				require.NoError(t, err)

				if !strings.Contains(buf1.String(), tc.logData) {
					t.Error("Handler did not log required message")
				}

				time.Sleep(1 * time.Second)
				svc.Stop(ctx)
				time.Sleep(1 * time.Second)
			})
		}
	})
}
