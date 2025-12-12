package profiler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/profiler"
)

func TestServer_StartIfEnabled(t *testing.T) {
	tests := []struct {
		name           string
		profilerEnable bool
		profilerPort   string
		expectRunning  bool
	}{
		{
			name:           "profiler disabled",
			profilerEnable: false,
			profilerPort:   ":6060",
			expectRunning:  false,
		},
		{
			name:           "profiler enabled with default port",
			profilerEnable: true,
			profilerPort:   ":6060",
			expectRunning:  true,
		},
		{
			name:           "profiler enabled with custom port",
			profilerEnable: true,
			profilerPort:   ":6061",
			expectRunning:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createTestConfig(tt.profilerEnable, tt.profilerPort)
			server := profiler.NewServer()
			ctx := t.Context()

			startProfilerAndVerify(ctx, t, server, cfg, tt.expectRunning)

			if tt.expectRunning {
				verifyPprofEndpoint(t, tt.profilerPort)
			}

			stopProfilerAndVerify(ctx, t, server)
		})
	}
}

func createTestConfig(enable bool, port string) *config.ConfigurationDefault {
	return &config.ConfigurationDefault{
		ProfilerEnable:   enable,
		ProfilerPortAddr: port,
	}
}

func startProfilerAndVerify(
	ctx context.Context,
	t *testing.T,
	server *profiler.Server,
	cfg *config.ConfigurationDefault,
	expectRunning bool,
) {
	err := server.StartIfEnabled(ctx, cfg)
	if err != nil {
		t.Errorf("unexpected error starting profiler: %v", err)
	}

	if expectRunning && !server.IsRunning() {
		t.Error("expected profiler server to be running when enabled")
	}
	if !expectRunning && server.IsRunning() {
		t.Error("expected profiler server to not be running when disabled")
	}
}

func verifyPprofEndpoint(t *testing.T, port string) {
	// Give the server a moment to start
	time.Sleep(200 * time.Millisecond)

	resp, httpErr := http.Get("http://localhost" + port + "/debug/pprof/")
	if httpErr != nil {
		t.Errorf("failed to connect to pprof server: %v", httpErr)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func stopProfilerAndVerify(ctx context.Context, t *testing.T, server *profiler.Server) {
	err := server.Stop(ctx)
	if err != nil {
		t.Errorf("unexpected error stopping profiler: %v", err)
	}

	if server.IsRunning() {
		t.Error("expected profiler server to be stopped after Stop()")
	}
}

func TestServer_Stop(t *testing.T) {
	server := profiler.NewServer()
	ctx := t.Context()

	// Test stopping when not running
	err := server.Stop(ctx)
	if err != nil {
		t.Errorf("unexpected error stopping non-running server: %v", err)
	}

	// Test stopping when running
	cfg := &config.ConfigurationDefault{
		ProfilerEnable:   true,
		ProfilerPortAddr: ":6062",
	}

	err = server.StartIfEnabled(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	err = server.Stop(ctx)
	if err != nil {
		t.Errorf("unexpected error stopping running server: %v", err)
	}

	if server.IsRunning() {
		t.Error("expected server to be stopped after Stop()")
	}
}
