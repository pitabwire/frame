package testoryhydra_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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
		pgDep := testpostgres.New()

		return []definition.TestResource{
			pgDep,
			testoryhydra.NewWithOpts(testoryhydra.HydraConfiguration, definition.WithDependancies(pgDep)),
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

	frametests.WithTestDependancies(h.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		testCases := []struct {
			name   string
			path   string
			status int
		}{
			{
				name:   "Liveness check to hydra",
				path:   "/health/alive",
				status: 200,
			},
			{
				name:   "Successfull ready hydra",
				path:   "/health/ready",
				status: 200,
			},
			{
				name:   "Straight to hydra",
				path:   "",
				status: 404,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				for _, res := range depOpt.All() {
					if strings.Contains(res.GetDS().String(), "http") {
						resp, err := http.Get(res.GetDS().String() + tc.path)
						h.NoError(err)

						defer resp.Body.Close() // Important to close the response body

						body, err := io.ReadAll(resp.Body)
						h.NoError(err)

						t.Log(string(body))

						h.Equal(tc.status, resp.StatusCode)
					}
				}
			})
		}
	})
}
