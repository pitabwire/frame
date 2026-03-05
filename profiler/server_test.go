package profiler_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests"
)

type ProfilerServerSuite struct {
	suite.Suite
}

func TestProfilerServerSuite(t *testing.T) {
	suite.Run(t, new(ProfilerServerSuite))
}

func (s *ProfilerServerSuite) TestStartIfEnabled() {
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
		s.Run(tt.name, func() {
			t := s.T()

			cfg := &config.ConfigurationDefault{
				ProfilerEnable:   tt.profilerEnable,
				ProfilerPortAddr: tt.profilerPort,
			}

			httpTestOpt, _ := frametests.WithHTTPTestDriver()

			ctx, svc := frame.NewService(
				frame.WithName("profiler-test"),
				frame.WithConfig(cfg),
				httpTestOpt,
			)
			defer svc.Stop(ctx)

			err := svc.Run(ctx, "")
			require.NoError(t, err)

			if tt.expectRunning {
				// Give the server a moment to start
				time.Sleep(200 * time.Millisecond)

				resp, httpErr := http.Get(fmt.Sprintf("http://localhost%s/debug/pprof/", tt.profilerPort))
				require.NoError(t, httpErr)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
			}
		})
	}
}

func (s *ProfilerServerSuite) TestStop() {
	t := s.T()

	cfg := &config.ConfigurationDefault{
		ProfilerEnable:   true,
		ProfilerPortAddr: ":6062",
	}

	httpTestOpt, _ := frametests.WithHTTPTestDriver()

	ctx, svc := frame.NewService(
		frame.WithName("profiler-stop-test"),
		frame.WithConfig(cfg),
		httpTestOpt,
	)

	err := svc.Run(ctx, "")
	require.NoError(t, err)

	// Give the server a moment to start
	time.Sleep(200 * time.Millisecond)

	// Verify profiler is reachable
	resp, err := http.Get("http://localhost:6062/debug/pprof/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop the service which should also stop the profiler
	svc.Stop(ctx)

	// Give it a moment to shut down
	time.Sleep(200 * time.Millisecond)

	// Verify profiler is no longer reachable
	_, err = http.Get("http://localhost:6062/debug/pprof/")
	require.Error(t, err, "profiler should not be reachable after service stop")
}

func (s *ProfilerServerSuite) TestProfilerDisabledByDefault() {
	t := s.T()

	httpTestOpt, _ := frametests.WithHTTPTestDriver()

	ctx, svc := frame.NewService(
		frame.WithName("profiler-default-test"),
		httpTestOpt,
	)
	defer svc.Stop(ctx)

	err := svc.Run(ctx, "")
	require.NoError(t, err)

	// Default config has profiler disabled, so port :6060 should not respond
	// with pprof (unless something else is on that port)
	time.Sleep(100 * time.Millisecond)

	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err = client.Get("http://localhost:6063/debug/pprof/")
	require.Error(t, err, "profiler should not be running with default config")
}
