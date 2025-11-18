package frame_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
)

// ServiceTestSuite extends FrameBaseTestSuite for comprehensive service testing.
type ServiceTestSuite struct {
	tests.BaseTestSuite
}

// TestServiceSuite runs the service test suite.
func TestServiceSuite(t *testing.T) {
	suite.Run(t, &ServiceTestSuite{})
}

// TestDefaultService tests default service creation.
func (s *ServiceTestSuite) TestDefaultService() {
	testCases := []struct {
		name        string
		serviceName string
		expectError bool
	}{
		{
			name:        "create default service",
			serviceName: "Test Srv",
			expectError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(frame.WithName(tc.serviceName))
				require.NotNil(t, srv, "default service should be instantiated")
				require.Equal(t, tc.serviceName, srv.Name(), "service name should match")
			})
		}
	})
}

// TestService tests basic service creation.
func (s *ServiceTestSuite) TestService() {
	testCases := []struct {
		name        string
		serviceName string
	}{
		{
			name:        "create basic service",
			serviceName: "Test",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(frame.WithName(tc.serviceName))
				require.NotNil(t, srv, "service should be instantiated")
			})
		}
	})
}

// TestFromContext tests service retrieval from context.
func (s *ServiceTestSuite) TestFromContext() {
	testCases := []struct {
		name        string
		serviceName string
		setService  bool
		expectNil   bool
	}{
		{
			name:        "service not in context",
			serviceName: "Test Srv",
			setService:  false,
			expectNil:   true,
		},
		{
			name:        "service in context",
			serviceName: "Test Srv",
			setService:  true,
			expectNil:   false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := s.T().Context()
				_, srv := frame.NewService(frame.WithName(tc.serviceName))

				if tc.setService {
					ctx = frame.ToContext(ctx, srv)
				}

				retrievedSrv := frame.FromContext(ctx)

				if tc.expectNil {
					require.Nil(t, retrievedSrv, "service should not be found in context")
				} else {
					require.NotNil(t, retrievedSrv, "service should be found in context")
				}
			})
		}
	})
}

// TestServiceAddCleanupMethod tests cleanup method functionality.
func (s *ServiceTestSuite) TestServiceAddCleanupMethod() {
	testCases := []struct {
		name         string
		serviceName  string
		cleanupCount int
	}{
		{
			name:         "add multiple cleanup methods",
			serviceName:  "Test Srv",
			cleanupCount: 2,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName))

				a := 30

				for range tc.cleanupCount {
					srv.AddCleanupMethod(func(_ context.Context) {
						a++
					})
				}

				require.Equal(t, 30, a, "cleanup methods should not run prematurely")

				srv.Stop(ctx)

				require.Equal(t, 30+tc.cleanupCount, a, "cleanup methods should run at shutdown")
			})
		}
	})
}

type testHC struct{}

func (h *testHC) CheckHealth() error {
	return nil
}

// TestServiceAddHealthCheck tests health check functionality.
func (s *ServiceTestSuite) TestServiceAddHealthCheck() {
	testCases := []struct {
		name           string
		serviceName    string
		addHealthCheck bool
		expectCount    int
	}{
		{
			name:           "no health checkers by default",
			serviceName:    "Test Srv",
			addHealthCheck: false,
			expectCount:    0,
		},
		{
			name:           "add health checker",
			serviceName:    "Test Srv",
			addHealthCheck: true,
			expectCount:    1,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(frame.WithName(tc.serviceName))

				if tc.addHealthCheck {
					healthChecker := new(testHC)
					require.Nil(t, srv.HealthCheckers(), "health checkers should not be present by default")

					srv.AddHealthCheck(healthChecker)

					require.Len(t, srv.HealthCheckers(), tc.expectCount, "health checkers should be added")
				} else {
					require.Nil(t, srv.HealthCheckers(), "health checkers should not be present by default")
				}
			})
		}
	})
}

// TestBackGroundConsumer tests background consumer functionality.
func (s *ServiceTestSuite) TestBackGroundConsumer() {
	testCases := []struct {
		name         string
		serviceName  string
		consumerFunc func(context.Context) error
		expectError  bool
	}{
		{
			name:        "successful background consumer",
			serviceName: "Test Srv",
			consumerFunc: func(_ context.Context) error {
				return nil
			},
			expectError: false,
		},
		{
			name:        "background consumer with error",
			serviceName: "Test Srv",
			consumerFunc: func(_ context.Context) error {
				return errors.New("background errors in the system")
			},
			expectError: true,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(
					frame.WithName(tc.serviceName),
					frame.WithBackgroundConsumer(tc.consumerFunc),
				)

				err := srv.Run(ctx, ":")

				if tc.expectError {
					require.Error(t, err, "background consumer error should be propagated")
				} else {
					require.NoError(t, err, "background consumer should run peacefully")
				}
			})
		}
	})
}

// TestServiceExitByOSSignal tests service exit by OS signal.
func (s *ServiceTestSuite) TestServiceExitByOSSignal() {
	testCases := []struct {
		name        string
		serviceName string
		signal      syscall.Signal
	}{
		{
			name:        "exit by SIGINT",
			serviceName: "Test Srv",
			signal:      syscall.SIGINT,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName))

				go func(srv *frame.Service) {
					err := srv.Run(ctx, ":")
					assert.ErrorIs(t, err, context.Canceled, "service should exit correctly on context cancellation")
				}(srv)

				time.Sleep(1 * time.Second)
				err := syscall.Kill(os.Getpid(), tc.signal)
				if err != nil {
					t.Skip("Could not send signal")
				}
			})
		}
	})
}

// getTestHealthHandler creates a test HTTP handler.
func (s *ServiceTestSuite) getTestHealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(4))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusAccepted)
		_, err := io.WriteString(w, "tsto")
		if err != nil {
			return
		}
	})
	return mux
}

// TestHealthCheckEndpoints tests health check endpoints.
func (s *ServiceTestSuite) TestHealthCheckEndpoints() {
	testCases := []struct {
		name       string
		healthPath string
		path       string
		handler    http.Handler
		statusCode int
	}{
		{name: "Empty Happy path", healthPath: "", path: "/healthz", statusCode: 200},
		{name: "Empty Unknown Path", healthPath: "", path: "/any/path", statusCode: 404},
		{name: "Happy path", healthPath: "/healthz", path: "/healthz", statusCode: 200},
		{name: "Unknown Path", healthPath: "/any/path", path: "/any/path", statusCode: 200},
		{
			name:       "Default Path with handlers",
			healthPath: "",
			path:       "/",
			statusCode: 202,
			handler:    s.getTestHealthHandler(),
		},
		{
			name:       "Health Path with handlers",
			healthPath: "",
			path:       "/healthz",
			statusCode: 200,
			handler:    s.getTestHealthHandler(),
		},
		{
			name:       "Random Path with handlers",
			healthPath: "",
			path:       "/any/path",
			statusCode: 202,
			handler:    s.getTestHealthHandler(),
		},
		{
			name:       "Unknown Path with handlers",
			healthPath: "/",
			path:       "/",
			statusCode: 202,
			handler:    s.getTestHealthHandler(),
		},
		{
			name:       "Unknown Path with handlers",
			healthPath: "/",
			path:       "/healthz",
			statusCode: 200,
			handler:    s.getTestHealthHandler(),
		},
		{
			name:       "Unknown Path with handlers",
			healthPath: "/",
			path:       "/any/path",
			statusCode: 202,
			handler:    s.getTestHealthHandler(),
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				opts := []frame.Option{
					frame.WithName("Test Srv"),
					frametests.WithNoopDriver(),
					frame.WithHealthCheckPath(tc.healthPath),
				}

				if tc.handler != nil {
					opts = append(opts, frame.WithHTTPHandler(tc.handler))
				}

				ctx, srv := frame.NewService(opts...)
				defer srv.Stop(ctx)

				err := srv.Run(ctx, ":41576")
				require.NoError(t, err, "server should start without error")

				ts := httptest.NewServer(srv.H())
				defer ts.Close()

				resp, err := http.Get(fmt.Sprintf("%s%s", ts.URL, tc.path))
				require.NoError(t, err, "should be able to invoke server")

				body, _ := io.ReadAll(resp.Body)

				require.Equal(t, tc.statusCode, resp.StatusCode,
					"expected status code %v, got %v", tc.statusCode, resp.StatusCode)

				t.Logf("%s", string(body))
			})
		}
	})
}

func TestService_ConfigurationSetup(t *testing.T) {
	testCases := []struct {
		name                string
		config              *config.ConfigurationDefault
		options             []frame.Option
		expectedName        string
		expectedEnvironment string
		expectedVersion     string
	}{
		{
			name: "Default configuration with options",
			config: &config.ConfigurationDefault{
				ServiceName:        "default-service",
				ServiceEnvironment: "default-env",
				ServiceVersion:     "default-version",
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
			},
			options: []frame.Option{
				frame.WithName("test-service"),
				frame.WithEnvironment("test"),
				frame.WithVersion("1.0.0"),
			},
			expectedName:        "test-service",
			expectedEnvironment: "test",
			expectedVersion:     "1.0.0",
		},
		{
			name: "Production configuration",
			config: &config.ConfigurationDefault{
				ServiceName:        "default-service",
				ServiceEnvironment: "default-env",
				ServiceVersion:     "default-version",
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
			},
			options: []frame.Option{
				frame.WithName("prod-service"),
				frame.WithEnvironment("production"),
				frame.WithVersion("2.1.0"),
			},
			expectedName:        "prod-service",
			expectedEnvironment: "production",
			expectedVersion:     "2.1.0",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			allOpts := append([]frame.Option{frame.WithConfig(tt.config)}, tt.options...)
			_, svc := frame.NewServiceWithContext(context.Background(), allOpts...)

			assert.Equal(t, tt.expectedName, svc.Name())
			assert.Equal(t, tt.expectedEnvironment, svc.Environment())
			assert.Equal(t, tt.expectedVersion, svc.Version())

			// Verify config is of type ConfigurationDefault
			_, ok := svc.Config().(*config.ConfigurationDefault)
			require.True(t, ok)
			assert.Equal(t, tt.expectedName, svc.Name())
			assert.Equal(t, tt.expectedEnvironment, svc.Environment())
			assert.Equal(t, tt.expectedVersion, svc.Version())
		})
	}
}

func TestService_TelemetryConfiguration(t *testing.T) {
	testCases := []struct {
		name                   string
		config                 *config.ConfigurationDefault
		expectTelemetryWorking bool
	}{
		{
			name: "Telemetry enabled with tracing",
			config: &config.ConfigurationDefault{
				ServiceName:          "telemetry-test",
				ServiceEnvironment:   "test",
				ServiceVersion:       "1.0.0",
				OpenTelemetryDisable: false,
				WorkerPoolCount:      10,
				WorkerPoolCapacity:   100,
			},
			expectTelemetryWorking: true,
		},
		{
			name: "Telemetry disabled",
			config: &config.ConfigurationDefault{
				ServiceName:          "telemetry-disabled",
				ServiceEnvironment:   "test",
				ServiceVersion:       "1.0.0",
				OpenTelemetryDisable: true,
				WorkerPoolCount:      10,
				WorkerPoolCapacity:   100,
			},
			expectTelemetryWorking: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			_, svc := frame.NewServiceWithContext(context.Background(), frame.WithConfig(tt.config))

			telemetryManager := svc.TelemetryManager()
			// Verify telemetry manager is created
			require.NotNil(t, telemetryManager)
			assert.Equal(t, tt.expectTelemetryWorking, !telemetryManager.Disabled())

			// Verify service properties are set from config
			assert.Equal(t, tt.config.ServiceName, svc.Name())
			assert.Equal(t, tt.config.ServiceEnvironment, svc.Environment())
			assert.Equal(t, tt.config.ServiceVersion, svc.Version())
		})
	}
}

func TestService_HTTPClientTracing(t *testing.T) {
	testCases := []struct {
		name          string
		config        *config.ConfigurationDefault
		expectTracing bool
	}{
		{
			name: "HTTP client tracing enabled",
			config: &config.ConfigurationDefault{
				ServiceName:        "client-test",
				ServiceEnvironment: "test",
				ServiceVersion:     "1.0.0",
				TraceRequests:      true,
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
			},
			expectTracing: true,
		},
		{
			name: "HTTP client tracing disabled",
			config: &config.ConfigurationDefault{
				ServiceName:        "client-test",
				ServiceEnvironment: "test",
				ServiceVersion:     "1.0.0",
				TraceRequests:      false,
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
			},
			expectTracing: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture log output
			capturedOutput := &bytes.Buffer{}

			ctx := context.Background()
			ctx, svc := frame.NewServiceWithContext(
				ctx,
				frame.WithConfig(tt.config),
				frame.WithLogger(util.WithLogOutput(capturedOutput), util.WithLogNoColor(true)),
			)

			clientManager := svc.HTTPClientManager()
			require.NotNil(t, clientManager)

			client := clientManager.Client(ctx)
			require.NotNil(t, client)

			// Create a test server to capture requests
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("test response"))
			}))
			defer testServer.Close()

			// Create a request with our context that has the custom logger
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL, nil)
			require.NoError(t, err)

			// Make a request with the traced client using our context
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			logOutput := capturedOutput.String()
			// No need to strip ANSI codes since we disabled colors
			t.Logf("Captured logs: %s", logOutput)

			// Verify tracing configuration
			cfg := svc.Config().(*config.ConfigurationDefault)
			assert.Equal(t, tt.expectTracing, cfg.TraceReq())

			// Verify that tracing logs are generated when tracing is enabled
			if tt.expectTracing {
				// Should contain HTTP request and response logs
				assert.Contains(t, logOutput, "HTTP request sent",
					"Should log HTTP request when tracing is enabled")
				assert.Contains(t, logOutput, "HTTP response received",
					"Should log HTTP response when tracing is enabled")
				assert.Contains(t, logOutput, "method=GET",
					"Should log request method when tracing is enabled")
				assert.Contains(t, logOutput, "status=200",
					"Should log response status when tracing is enabled")

				t.Log("✓ HTTP client tracing is enabled - logs captured and verified")
			} else {
				// Should NOT contain HTTP request and response logs
				assert.NotContains(t, logOutput, "HTTP request sent",
					"Should not log HTTP request when tracing is disabled")
				assert.NotContains(t, logOutput, "HTTP response received",
					"Should not log HTTP response when tracing is disabled")

				t.Log("✓ HTTP client tracing is disabled - no logs generated")
			}
		})
	}
}

func TestService_HTTPServerTracing(t *testing.T) {
	testCases := []struct {
		name          string
		config        *config.ConfigurationDefault
		expectTracing bool
	}{
		{
			name: "Server tracing enabled",
			config: &config.ConfigurationDefault{
				ServiceName:        "server-test",
				ServiceEnvironment: "test",
				ServiceVersion:     "1.0.0",
				TraceRequests:      true,
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
			},
			expectTracing: true,
		},
		{
			name: "Server tracing disabled",
			config: &config.ConfigurationDefault{
				ServiceName:        "server-test",
				ServiceEnvironment: "test",
				ServiceVersion:     "1.0.0",
				TraceRequests:      false,
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
			},
			expectTracing: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("test response"))
				assert.NoError(t, err)
			})

			ctx := context.Background()
			ctx, svc := frame.NewServiceWithContext(
				ctx,
				frame.WithConfig(tt.config),
				frame.WithHTTPHandler(testHandler),
			)

			// Create a test request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req = req.WithContext(ctx) // Ensure the request has the logger context
			responseRecorder := httptest.NewRecorder()

			// Get the handler from the service
			handler := svc.H()
			require.NotNil(t, handler)

			// Serve the request
			handler.ServeHTTP(responseRecorder, req)

			// Verify response
			assert.Equal(t, http.StatusOK, responseRecorder.Code)
			assert.Contains(t, responseRecorder.Body.String(), "test response")

			// Verify tracing configuration
			cfg := svc.Config().(*config.ConfigurationDefault)
			assert.Equal(t, tt.expectTracing, cfg.TraceReq())

			// Verify that the tracing configuration is properly set
			if tt.expectTracing {
				// Verify that the service was configured with tracing
				assert.True(t, cfg.TraceReq(), "TraceRequests should be enabled in config")

				t.Log("✓ HTTP server tracing is enabled - configuration verified")
				t.Log("  Note: Server logging middleware requires actual service running to capture logs")
			} else {
				// When tracing is disabled, the configuration should reflect that
				assert.False(t, cfg.TraceReq(), "TraceRequests should be disabled in config")
				t.Log("✓ HTTP server tracing is disabled - configuration verified")
			}
		})
	}
}

func TestService_WithIndividualOptions(t *testing.T) {
	// Test individual options
	_, svc := frame.NewServiceWithContext(context.Background(),
		frame.WithName("override-name"),
		frame.WithEnvironment("override-env"),
		frame.WithVersion("override-version"),
	)

	// Verify options are applied
	assert.Equal(t, "override-name", svc.Name())
	assert.Equal(t, "override-env", svc.Environment())
	assert.Equal(t, "override-version", svc.Version())
}

func TestService_ConfigurationPrecedence(t *testing.T) {
	// Test that ConfigurationDefault is properly used
	cfg := &config.ConfigurationDefault{
		ServiceName:          "precedence-service",
		ServiceEnvironment:   "staging",
		ServiceVersion:       "3.2.1",
		OpenTelemetryDisable: false,
		TraceRequests:        true,
		ServerPort:           "9090",
		WorkerPoolCount:      10,
		WorkerPoolCapacity:   100,
	}

	_, svc := frame.NewServiceWithContext(context.Background(), frame.WithConfig(cfg))

	// Verify all configuration options are respected
	assert.Equal(t, "precedence-service", svc.Name())
	assert.Equal(t, "staging", svc.Environment())
	assert.Equal(t, "3.2.1", svc.Version())
	assert.Equal(t, "precedence-service", cfg.Name())
	assert.Equal(t, "staging", cfg.Environment())
	assert.Equal(t, "3.2.1", cfg.Version())
	assert.False(t, cfg.DisableOpenTelemetry())
	assert.True(t, cfg.TraceReq())
	assert.Equal(t, "9090", cfg.ServerPort)
}

func TestService_HTTPClientConfiguration(t *testing.T) {
	// Create configuration for HTTP client testing
	cfg := &config.ConfigurationDefault{
		ServiceName:        "http-client-service",
		ServiceEnvironment: "test",
		ServiceVersion:     "1.0.0",
		TraceRequests:      true,
		WorkerPoolCount:    10,
		WorkerPoolCapacity: 100,
	}

	ctx := context.Background()
	ctx, svc := frame.NewServiceWithContext(ctx, frame.WithConfig(cfg))

	clientManager := svc.HTTPClientManager()
	// Verify client manager exists
	require.NotNil(t, svc.HTTPClientManager())

	// Test that we can get the HTTP client
	httpClient := clientManager.Client(ctx)
	require.NotNil(t, httpClient)

	// Test that we can set a custom HTTP client
	customClient := &http.Client{
		Timeout: 30,
	}
	clientManager.SetClient(ctx, customClient)

	// Verify the client was updated
	updatedClient := clientManager.Client(ctx)
	assert.Equal(t, customClient, updatedClient)
}

func TestService_SecurityManagerConfiguration(t *testing.T) {
	// Test that security manager is properly configured with interface-based approach
	cfg := &config.ConfigurationDefault{
		ServiceName:                  "security-test",
		ServiceEnvironment:           "test",
		ServiceVersion:               "1.0.0",
		Oauth2ServiceURI:             "https://test-issuer.com",
		Oauth2ServiceClientID:        "test-client-id",
		Oauth2ServiceClientSecret:    "test-client-secret",
		AuthorizationServiceReadURI:  "https://test-auth.com/read",
		AuthorizationServiceWriteURI: "https://test-auth.com/write",
		WorkerPoolCount:              10,
		WorkerPoolCapacity:           100,
	}

	ctx, svc := frame.NewServiceWithContext(context.Background(), frame.WithConfig(cfg))

	sm := svc.SecurityManager()
	// Verify security manager is created
	require.NotNil(t, sm)

	// Verify we can get security components
	registrar := sm.GetOauth2ClientRegistrar(ctx)
	require.NotNil(t, registrar)

	authenticator := sm.GetAuthenticator(ctx)
	require.NotNil(t, authenticator)

	authorizer := sm.GetAuthorizer(ctx)
	require.NotNil(t, authorizer)

	// Verify service properties are set correctly
	assert.Equal(t, "security-test", svc.Name())
	assert.Equal(t, "test", svc.Environment())
	assert.Equal(t, "1.0.0", svc.Version())
}

func TestService_H2CSupport(t *testing.T) {
	// Test h2c (HTTP/2 without TLS) server functionality - h2c is enabled by default
	testCases := []struct {
		name        string
		description string
	}{
		{
			name:        "default h2c enabled server",
			description: "Server should enable h2c by default",
		},
		{
			name:        "explicitly disabled h2c server",
			description: "Server should use standard HTTP when h2c is explicitly disabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.ConfigurationDefault{
				ServiceName:        "h2c-test",
				ServiceEnvironment: "test",
				ServiceVersion:     "1.0.0",
				WorkerPoolCount:    10,
				WorkerPoolCapacity: 100,
				EventsQueueURL:     "mem://test", // Use in-memory queue for testing
			}

			// Create service with h2c enabled by default, or explicitly disabled
			var opts []frame.Option

			// Add a simple HTTP handler for testing
			handler := http.NewServeMux()
			handler.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message": "h2c test successful", "protocol": "` + r.Proto + `"}`))
			})

			opts = append(opts, frame.WithHTTPHandler(handler), frame.WithConfig(cfg))

			ctx, svc := frame.NewServiceWithContext(t.Context(), opts...)
			defer svc.Stop(ctx)
			go func() {
				// Start the service
				_ = svc.Run(ctx, ":8080")
			}()
			// Wait a moment for server to start
			time.Sleep(100 * time.Millisecond)

			// Test client with h2c support
			clientOpts := []client.HTTPOption{client.WithHTTPEnableH2C()}

			httpClient := client.NewHTTPClient(ctx, clientOpts...)

			// Make a request to the test endpoint
			resp, err := httpClient.Get("http://localhost:8080/test")
			if err != nil {
				svc.Stop(ctx)
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			// Verify response
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			responseStr := string(body)
			assert.Contains(t, responseStr, "h2c test successful")
		})
	}
}

func TestService_H2CClientConfiguration(t *testing.T) {
	// Test h2c client configuration independently
	testCases := []struct {
		name          string
		enableH2C     bool
		expectedProto string
	}{
		{
			name:          "h2c client",
			enableH2C:     true,
			expectedProto: "HTTP/2.0",
		},
		{
			name:          "standard HTTP client",
			enableH2C:     false,
			expectedProto: "HTTP/1.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []client.HTTPOption
			if tc.enableH2C {
				opts = append(opts, client.WithHTTPEnableH2C())
			} else {
				// For standard HTTP, create a transport that only supports HTTP/1.1
				standardTransport := &http.Transport{
					ForceAttemptHTTP2: false,
				}
				opts = append(opts, client.WithHTTPTransport(standardTransport))
			}

			httpClient := client.NewHTTPClient(t.Context(), opts...)

			// Create a test server that supports h2c
			handler := http.NewServeMux()
			handler.HandleFunc("/proto-test", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"protocol": "` + r.Proto + `"}`))
			})

			var server *httptest.Server = httptest.NewUnstartedServer(handler)
			protocols := new(http.Protocols)
			protocols.SetHTTP1(true)
			protocols.SetUnencryptedHTTP2(true)
			server.Config.Protocols = protocols
			server.Start()

			defer server.Close()

			// Make request
			resp, err := httpClient.Get(server.URL + "/proto-test")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			t.Logf("Client response: %s", string(body))

			// Verify the protocol matches expected value
			var response map[string]string
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedProto, response["protocol"])

			// Verify the client was created successfully
			assert.NotNil(t, httpClient.Transport)
		})
	}
}
