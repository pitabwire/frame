package frametests_test

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/frametests"
	"github.com/stretchr/testify/require"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
}

func TestBoundHTTPTestDriver_ServesOnConfiguredPort(t *testing.T) {
	port, err := frametests.GetFreePort(t.Context())
	require.NoError(t, err)

	driverOpt, testServer := frametests.WithBoundHTTPTestDriver()
	cfg := config.ConfigurationDefault{HTTPServerPort: strconv.Itoa(port)}
	ctx, svc := frame.NewServiceWithContext(t.Context(),
		frame.WithName("bound-driver"),
		frame.WithConfig(&cfg),
		driverOpt,
		frame.WithHTTPHandler(okHandler()),
	)
	defer svc.Stop(ctx)

	require.NoError(t, svc.Run(ctx, ""), "Run returns once the listener is bound")
	require.NotNil(t, testServer())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	require.NoError(t, err)
	resp, err := testServer().Client().Do(req)
	require.NoError(t, err, "service must be reachable on the pre-allocated port")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func TestBoundHTTPTestDriver_ReturnsListenErrorFromRun(t *testing.T) {
	// Occupy a port so binding it must fail.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = occupied.Close() }()
	addr := occupied.Addr().String()

	driverOpt, _ := frametests.WithBoundHTTPTestDriver()
	ctx, svc := frame.NewServiceWithContext(t.Context(),
		frame.WithName("bound-driver-conflict"),
		driverOpt,
		frame.WithHTTPHandler(okHandler()),
	)
	defer svc.Stop(ctx)

	err = svc.Run(ctx, addr)
	require.Error(t, err, "a listen failure must surface from Run, not be lost")
	require.ErrorContains(t, err, addr)
}

func TestHTTPTestDriver_StillPicksRandomPort(t *testing.T) {
	driverOpt, testServer := frametests.WithHTTPTestDriver()
	ctx, svc := frame.NewServiceWithContext(t.Context(),
		frame.WithName("random-driver"),
		driverOpt,
		frame.WithHTTPHandler(okHandler()),
	)
	defer svc.Stop(ctx)

	require.NoError(t, svc.Run(ctx, ":41577"))
	require.NotNil(t, testServer())
	require.NotContains(t, testServer().URL, ":41577", "random-port driver must ignore the configured address")
}
