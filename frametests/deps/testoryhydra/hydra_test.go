package testoryhydra_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testoryhydra"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
)

// HydraImageSetupTestSuite extends FrameBaseTestSuite for comprehensive search testing.
type HydraImageSetupTestSuite struct {
	frametests.FrameBaseTestSuite
}

func (h *HydraImageSetupTestSuite) SetupSuite() {
	h.InitResourceFunc = func(_ context.Context) []definition.TestResource {
		pgDep := testpostgres.NewWithOpts(testpostgres.DBName,
			definition.WithEnableLogging(true),
			definition.WithUseHostMode(false))

		return []definition.TestResource{
			pgDep,
			testoryhydra.NewWithOpts(
				testoryhydra.HydraConfiguration,
				definition.WithDependancies(pgDep),
				definition.WithEnableLogging(true),
				definition.WithUseHostMode(false),
			),
		}
	}

	h.FrameBaseTestSuite.SetupSuite()
}

func TestHydraImageSetup(t *testing.T) {
	suite.Run(t, new(HydraImageSetupTestSuite))
}

// TestHydraImageSetup tests the hydra image setup.
func (h *HydraImageSetupTestSuite) TestHydraImageSetup() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("hydra setup", "hydra_t", h.Resources()),
	}

	testCases := []struct {
		name      string
		path      string
		portToUse string
		status    int
	}{
		{
			name:      "Liveness check to hydra",
			path:      "/health/alive",
			portToUse: "4445/tcp",
			status:    200,
		},
		{
			name:      "Successfull ready hydra",
			path:      "/health/ready",
			portToUse: "4445/tcp",
			status:    200,
		},
		{
			name:      "Straight to hydra admin",
			path:      "",
			portToUse: "4445/tcp",
			status:    404,
		},
		{
			name:      "Straight to hydra",
			path:      "",
			portToUse: "4444/tcp",
			status:    404,
		},
		{
			name:      "Open ID Configuration admin",
			path:      "/.well-known/openid-configuration",
			portToUse: "4445/tcp",
			status:    404,
		},
		{
			name:      "Open ID Configuration",
			path:      "/.well-known/openid-configuration",
			portToUse: "4444/tcp",
			status:    200,
		},
		{
			name:      "Json Web Keys path",
			path:      "/.well-known/jwks.json",
			portToUse: "4444/tcp",
			status:    200,
		},
	}

	frametests.WithTestDependancies(h.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		depCon := depOpt.ByImageName(testoryhydra.OryHydraImage)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()

				ds := depCon.GetDS(ctx)
				portMapping, err := depCon.PortMapping(ctx, tc.portToUse)
				require.NoError(t, err)

				ds, err = ds.ChangePort(portMapping)
				require.NoError(t, err)

				resp, err := http.Get(ds.String() + tc.path)
				require.NoError(t, err)

				defer util.CloseAndLogOnError(ctx, resp.Body) // Important to close the response body

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				t.Log(string(body))

				require.Equalf(
					t,
					tc.status,
					resp.StatusCode,
					"expected status code to be %d but got %d",
					tc.status,
					resp.StatusCode,
				)
			})
		}
	})
}
