package frame_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/pitabwire/frame"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestDefaultService(t *testing.T) {

	_, srv := frame.NewService("Test Srv")
	if srv == nil {
		t.Errorf("No default service could be instaniated")
		return
	}

	if srv.Name() != "Test Srv" {
		t.Errorf("s")
	}
}

func TestService(t *testing.T) {

	_, srv := frame.NewService("Test")
	if srv == nil {
		t.Errorf("No default service could be instaniated")
	}
}

func TestFromContext(t *testing.T) {

	ctx := context.Background()

	_, srv := frame.NewService("Test Srv")

	nullSrv := frame.FromContext(ctx)
	if nullSrv != nil {
		t.Errorf("Service was found in context yet it was not set")
	}

	ctx = frame.ToContext(ctx, srv)

	valueSrv := frame.FromContext(ctx)
	if valueSrv == nil {
		t.Errorf("No default service was found in context")
	}

}

func TestService_AddCleanupMethod(t *testing.T) {

	ctx, srv := frame.NewService("Test Srv")

	a := 30

	srv.AddCleanupMethod(func(ctx context.Context) {
		a++
	})

	srv.AddCleanupMethod(func(ctx context.Context) {
		a++
	})

	if a != 30 {
		t.Errorf("Clean up method is running prematurely")
	}

	srv.Stop(ctx)

	if a != 32 {
		t.Errorf("Clean up method is not running at shutdown")
	}
}

type testHC struct {
}

func (h *testHC) CheckHealth() error {
	return nil
}

func TestService_AddHealthCheck(t *testing.T) {

	_, srv := frame.NewService("Test Srv")

	healthChecker := new(testHC)

	if srv.HealthCheckers() != nil {
		t.Errorf("Health checkers are not supposed to be added by default")
	}

	srv.AddHealthCheck(healthChecker)

	if len(srv.HealthCheckers()) == 0 {
		t.Errorf("Health checkers are not being added to list")
	}
}

func TestBackGroundConsumer(t *testing.T) {

	listener := bufconn.Listen(1024 * 1024)

	ctx, srv := frame.NewService("Test Srv",
		frame.ServerListener(listener),
		frame.BackGroundConsumer(func(ctx context.Context) error {
			return nil
		}))

	err := srv.Run(ctx, ":")
	if err != nil {
		t.Errorf("could not start a background consumer peacefully : %v", err)
	}

	ctx, srv = frame.NewService("Test Srv", frame.BackGroundConsumer(func(ctx context.Context) error {
		return errors.New("background errors in the system")
	}))

	err = srv.Run(ctx, ":")
	if err == nil {
		t.Errorf("could not propagate background consumer error correctly")
	}

}

func TestServiceExitByOSSignal(t *testing.T) {

	listener := bufconn.Listen(1024 * 1024)

	ctx, srv := frame.NewService("Test Srv",
		frame.ServerListener(listener))

	go func(srv *frame.Service) {
		err := srv.Run(ctx, ":")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("service is not exiting correctly as the context is still not done")
		}
	}(srv)

	time.Sleep(1 * time.Second)
	err := syscall.Kill(os.Getpid(), syscall.SIGINT)
	if err != nil {
		return
	}

}

func getTestHealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
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

func TestHealthCheckEndpoints(t *testing.T) {
	tests := []struct {
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
		{name: "Default Path with handler", healthPath: "", path: "/", statusCode: 202, handler: getTestHealthHandler()},
		{name: "Health Path with handler", healthPath: "", path: "/healthz", statusCode: 200, handler: getTestHealthHandler()},
		{name: "Random Path with handler", healthPath: "", path: "/any/path", statusCode: 202, handler: getTestHealthHandler()},
		{name: "Unknown Path with handler", healthPath: "/", path: "/", statusCode: 202, handler: getTestHealthHandler()},
		{name: "Unknown Path with handler", healthPath: "/", path: "/healthz", statusCode: 200, handler: getTestHealthHandler()},
		{name: "Unknown Path with handler", healthPath: "/", path: "/any/path", statusCode: 202, handler: getTestHealthHandler()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			opts := []frame.Option{frame.NoopDriver(), frame.HealthCheckPath(test.healthPath)}

			if test.handler != nil {
				opts = append(opts, frame.HttpHandler(test.handler))
			}

			ctx, srv := frame.NewService("Test Srv", opts...)
			defer srv.Stop(ctx)

			err := srv.Run(ctx, ":41576")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			ts := httptest.NewServer(srv.H())
			defer ts.Close()

			resp, err := http.Get(fmt.Sprintf("%s%s", ts.URL, test.path))
			if err != nil {
				t.Errorf("could not invoke server %v", err)
				log.Fatal(err)
			}

			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != test.statusCode {
				t.Errorf("%v : expected status code %v is not %v", test.name, test.statusCode, resp.StatusCode)
			}

			fmt.Println(string(body))

		})
	}
}
