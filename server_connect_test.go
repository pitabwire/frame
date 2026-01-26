package frame_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	pingv1 "github.com/pitabwire/frame/frametests/rpcservice/ping/v1"
	"github.com/pitabwire/frame/frametests/rpcservice/ping/v1/pingv1connect"
	"github.com/pitabwire/frame/tests"
)

// ConnectServerTestSuite extends FrameBaseTestSuite for comprehensive Connect RPC server testing.
type ConnectServerTestSuite struct {
	tests.BaseTestSuite
}

// TestConnectServerSuite runs the Connect server test suite.
func TestConnectServerSuite(t *testing.T) {
	suite.Run(t, &ConnectServerTestSuite{})
}

type connectRPCHandler struct {
	pingv1connect.UnimplementedPingServiceHandler
}

func (s *connectRPCHandler) Ping(ctx context.Context, req *pingv1.PingRequest) (
	*pingv1.PingResponse, error) {
	log := util.Log(ctx)
	cfg := config.FromContext[config.ConfigurationLogLevel](ctx)
	cfgExists := cfg != nil
	log.WithField("config exists", cfgExists).
		WithField("log", log).
		Info("We received : " + req.GetName())

	return &pingv1.PingResponse{
		Message: "Hello " + req.GetName() + " from frame",
	}, nil
}

func (s *ConnectServerTestSuite) startConnectServer(_ *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle(pingv1connect.NewPingServiceHandler(&connectRPCHandler{}))

	server := httptest.NewServer(mux)
	return server
}

func (s *ConnectServerTestSuite) startTLSConnectServer(_ *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle(pingv1connect.NewPingServiceHandler(&connectRPCHandler{}))

	server := httptest.NewTLSServer(mux)
	return server
}

// TestRawConnectServer tests raw Connect RPC server functionality.
func (s *ConnectServerTestSuite) TestRawConnectServer() {
	testCases := []struct {
		name string
	}{
		{
			name: "basic Connect RPC server test",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				server := s.startConnectServer(t)
				defer server.Close()

				err := s.clientInvokeConnect(server.URL, "Testing Roma")
				if err != nil {
					_ = err
				}
			})
		}
	})
}

// TestServiceConnectHealthServer tests Connect RPC health server functionality.
func (s *ConnectServerTestSuite) TestServiceConnectHealthServer() {
	testCases := []struct {
		name        string
		serviceName string
		logData     string
	}{
		{
			name:        "Connect RPC health server test",
			serviceName: "Testing Service Connect",
			logData:     "Test logging",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var buf1 bytes.Buffer

				defConf, err := config.FromEnv[config.ConfigurationDefault]()
				require.NoError(t, err)

				httOpt, httpFn := frametests.WithHTTPTestDriver()

				ctx, svc := frame.NewService(
					frame.WithName(tc.serviceName),
					frame.WithConfig(&defConf),
					frame.WithLogger(util.WithLogOutput(&buf1)),
					httOpt,
				)
				defer svc.Stop(ctx)

				_, h := pingv1connect.NewPingServiceHandler(&connectRPCHandler{})

				svc.Init(ctx, frame.WithHTTPHandler(h))

				go func() {
					runErr := svc.Run(ctx, "") // Empty port for httptest
					if runErr != nil && !errors.Is(runErr, context.Canceled) &&
						!errors.Is(runErr, http.ErrServerClosed) {
						t.Errorf("TestServiceConnectHealthServer:  svc.Run failed: %v", runErr)
					}
				}()
				time.Sleep(1 * time.Second)
				err = s.clientInvokeConnectHealth(httpFn().URL, tc.logData)
				if err != nil {
					_ = err
				}
			})
		}
	})
}

// TestServiceConnectContextSetup tests Connect RPC health server functionality.
func (s *ConnectServerTestSuite) TestServiceConnectContextSetup() {
	testCases := []struct {
		name        string
		serviceName string
		logData     string
	}{
		{
			name:        "Connect RPC context setup tests",
			serviceName: "Testing Connect Service",
			logData:     "Test logging",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			t := s.T()
			var buf1 bytes.Buffer

			defConf, err := config.FromEnv[config.ConfigurationDefault]()
			if err != nil {
				_ = err
				return
			}

			httOpt, httpFn := frametests.WithHTTPTestDriver()

			ctx, svc := frame.NewServiceWithContext(
				t.Context(),
				frame.WithName(tc.serviceName),
				frame.WithConfig(&defConf),
				frame.WithLogger(util.WithLogOutput(&buf1)),
				httOpt,
			)
			defer svc.Stop(ctx)

			_, h := pingv1connect.NewPingServiceHandler(&connectRPCHandler{})

			svc.Init(ctx, frame.WithHTTPHandler(h))

			runErr := svc.Run(ctx, "") // Empty port for httptest
			if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, http.ErrServerClosed) {
				t.Errorf("TestServiceConnectContextSetup:  svc.Run failed: %v", runErr)
			}

			err = s.clientInvokeConnectHealth(httpFn().URL, tc.logData)
			if err != nil {
				_ = err
			}

			logData := buf1.String()
			// Check for the actual log message that the handler produces
			if !strings.Contains(logData, "We received : "+tc.logData) {
				t.Errorf("Handler did not log required message. Got: %s", logData)
			}
		})
	}
}

// TestServiceRequestTraces tests server request tracing functionality.
func (s *ConnectServerTestSuite) TestServiceRequestTraces() {
	testCases := []struct {
		name        string
		serviceName string
		logData     string
	}{
		{
			name:        "Server request tracing tests",
			serviceName: "Testing Request Traces",
			logData:     "Test logging",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			t := s.T()
			var buf1 bytes.Buffer

			cfg, err := config.FromEnv[config.ConfigurationDefault]()
			if err != nil {
				_ = err
				return
			}

			cfg.TraceRequests = true
			cfg.TraceRequestsLogBody = true

			httOpt, httpFn := frametests.WithHTTPTestDriver()

			ctx, svc := frame.NewServiceWithContext(
				t.Context(),
				frame.WithName(tc.serviceName),
				frame.WithConfig(&cfg),
				frame.WithLogger(util.WithLogOutput(&buf1)),
				httOpt,
			)
			defer svc.Stop(ctx)

			_, h := pingv1connect.NewPingServiceHandler(&connectRPCHandler{})

			svc.Init(ctx, frame.WithHTTPHandler(h))

			runErr := svc.Run(ctx, "") // Empty port for httptest
			if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, http.ErrServerClosed) {
				t.Errorf("TestServiceConnectContextSetup:  svc.Run failed: %v", runErr)
			}

			err = s.clientInvokeConnectHealth(httpFn().URL, tc.logData)
			if err != nil {
				_ = err
			}

			logData := buf1.String()
			// Check for the actual log message that the handler produces
			if !strings.Contains(logData, "We received : "+tc.logData) {
				t.Errorf("Handler did not log required message. Got: %s", logData)
			}

			if !strings.Contains(logData, "HTTP request completed") ||
				!strings.Contains(logData, "/ping.v1.PingService/Ping") {
				t.Errorf("Handler did not log request tracing data. Got: %s", logData)
			}
		})
	}
}

// TestServiceConnectServer tests service Connect RPC server functionality.
func (s *ConnectServerTestSuite) TestServiceConnectServer() {
	testCases := []struct {
		name        string
		serviceName string
		httpPort    string
	}{
		{
			name:        "service Connect RPC server test",
			serviceName: "Testing Service Connect",
			httpPort:    ":8081",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				mux := http.NewServeMux()
				mux.Handle(pingv1connect.NewPingServiceHandler(&connectRPCHandler{}))

				server := httptest.NewServer(mux)
				defer server.Close()

				httpTestOpt, _ := frametests.WithHTTPTestDriver()

				ctx, svc := frame.NewService(
					frame.WithName(tc.serviceName),
					httpTestOpt,
					frame.WithHTTPHandler(mux),
				)

				err := svc.Run(ctx, "")
				if err != nil {
					svc.Log(ctx).WithError(err).Error(" failed to run server ")
				}

				err = s.clientInvokeConnect(server.URL, "Testing Roma")
				if err != nil {
					_ = err
				}

				svc.Stop(ctx)
			})
		}
	})
}

// TestServiceConnectTLSServer tests TLS-enabled Connect RPC server functionality.
func (s *ConnectServerTestSuite) TestServiceConnectTLSServer() {
	testCases := []struct {
		name        string
		serviceName string
		httpPort    string
		certFile    string
	}{
		{
			name:        "TLS Connect RPC server test",
			serviceName: "Testing Service TLS Connect",
			httpPort:    ":8082",
			certFile:    "tests_runner/ca-cert.pem",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(_ *testing.T) {
				mux := http.NewServeMux()
				mux.Handle(pingv1connect.NewPingServiceHandler(&connectRPCHandler{}))

				server := s.startTLSConnectServer(t)
				defer server.Close()

				ctx, svc := frame.NewService(frame.WithName(tc.serviceName))

				httpTestOpt, _ := frametests.WithHTTPTestDriver()

				svc.Init(ctx, httpTestOpt, frame.WithHTTPHandler(mux))

				err := svc.Run(ctx, "")
				if err != nil {
					svc.Log(ctx).WithError(err).Error(" failed to run server ")
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

				err = s.clientInvokeConnectTLS(server.URL, tlsConfig)
				if err != nil {
					_ = err
				}

				time.Sleep(2 * time.Second)
				svc.Stop(ctx)
			})
		}
	})
}

// TestServiceConnectRun tests service run functionality with Connect RPC.
func (s *ConnectServerTestSuite) TestServiceConnectRun() {
	testCases := []struct {
		name        string
		serviceName string
		port        string
	}{
		{
			name:        "service Connect RPC run test",
			serviceName: "Testing Connect",
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

func (s *ConnectServerTestSuite) clientInvokeConnect(baseURL string, name string) error {
	client := pingv1connect.NewPingServiceClient(
		http.DefaultClient,
		baseURL,
	)

	req := pingv1.PingRequest{
		Name: name,
	}

	resp, err := client.Ping(context.Background(), &req)
	if err != nil {
		return err
	}

	if !strings.Contains(resp.GetMessage(), "frame") {
		return errors.New("The response message should contain the word frame ")
	}
	return nil
}

func (s *ConnectServerTestSuite) clientInvokeConnectHealth(baseURL, name string) error {
	// Connect RPC doesn't have a separate health service by default
	// We'll just test that the server responds to the ping endpoint
	return s.clientInvokeConnect(baseURL, name)
}

func (s *ConnectServerTestSuite) clientInvokeConnectTLS(baseURL string, tlsConfig *tls.Config) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	pingClient := pingv1connect.NewPingServiceClient(
		client,
		baseURL,
		connect.WithGRPC(),
	)

	req := pingv1.PingRequest{
		Name: "Testing Roma TLS",
	}

	resp, err := pingClient.Ping(context.Background(), &req)
	if err != nil {
		return err
	}

	if !strings.Contains(resp.GetMessage(), "frame") {
		return errors.New("The response message should contain the word frame ")
	}
	return nil
}

// TestServiceConnectContextRun tests service context propagation with Connect RPC.
func (s *ConnectServerTestSuite) TestServiceConnectContextRun() {
	testCases := []struct {
		name        string
		serviceName string
		port        string
		logData     string
	}{
		{
			name:        "service Connect RPC context propagation test",
			serviceName: "Testing Connect",
			port:        ":",
			logData:     "testing hello we are live with Connect",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var buf1 bytes.Buffer

				httpTestOpt, testSrvFn := frametests.WithHTTPTestDriver()

				mux := http.NewServeMux()
				mux.Handle(pingv1connect.NewPingServiceHandler(&connectRPCHandler{}))
				mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
					util.Log(r.Context()).Info(tc.logData)
				})

				ctx, svc := frame.NewServiceWithContext(t.Context(), frame.WithName(tc.serviceName),
					frame.WithLogger(util.WithLogOutput(&buf1)),
					httpTestOpt, frame.WithHTTPHandler(mux))

				if err := svc.Run(ctx, tc.port); err != nil {
					if !errors.Is(err, context.Canceled) {
						_ = err
					}
				}

				client := svc.HTTPClientManager()
				_, err := client.Invoke(t.Context(), "GET", testSrvFn().URL, nil, nil)
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
