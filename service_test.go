package frame_test

import (
	"context"
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(tc.serviceName)
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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(tc.serviceName)
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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := s.T().Context()
				_, srv := frame.NewService(tc.serviceName)

				if tc.setService {
					ctx = frame.SvcToContext(ctx, srv)
				}

				retrievedSrv := frame.Svc(ctx)

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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(tc.serviceName)

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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, srv := frame.NewService(tc.serviceName)

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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(tc.serviceName, frame.WithBackgroundConsumer(tc.consumerFunc))

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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(tc.serviceName)

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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				opts := []frame.Option{frametests.WithNoopDriver(), frame.WithHealthCheckPath(tc.healthPath)}

				if tc.handler != nil {
					opts = append(opts, frame.WithHTTPHandler(tc.handler))
				}

				ctx, srv := frame.NewService("Test Srv", opts...)
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
