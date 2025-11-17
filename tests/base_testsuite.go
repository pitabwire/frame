package tests

import (
	"context"
	"testing"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testoryhydra"
	"github.com/pitabwire/frame/frametests/deps/testoryketo"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
)

const (
	DefaultRandomStringLength = 8
)

type BaseTestSuite struct {
	frametests.FrameBaseTestSuite
}

func initResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("frame_test_service",
		definition.WithUserName("ant"), definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false), definition.WithUseHostMode(false))

	queue := testnats.NewWithOpts("partition",
		definition.WithUserName("ant"),
		definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false))

	resources := []definition.TestResource{pg, queue}
	return resources
}

func (bs *BaseTestSuite) SetupSuite() {
	if bs.InitResourceFunc == nil {
		bs.InitResourceFunc = initResources
	}
	bs.FrameBaseTestSuite.SetupSuite()
}

// WithTestDependancies Creates subtests with each known DependencyOption.
func (bs *BaseTestSuite) WithTestDependancies(
	t *testing.T,
	testFn func(t *testing.T, dep *definition.DependencyOption),
) {
	var allDeps []definition.DependancyConn
	var queueDR definition.DependancyConn

	// var authenticationDR definition.DependancyConn
	for _, res := range bs.Resources() {
		switch res.Name() {
		case testpostgres.PostgresqlDBImage:
			allDeps = append(allDeps, res)
		case testnats.NatsImage:
			queueDR = res
		// case internaltests.AuthenticationImage:
		// 	authenticationDR = res
		case testoryhydra.OryHydraImage:
			allDeps = append(allDeps, res)
		case testoryketo.OryKetoImage:
			allDeps = append(allDeps, res)
		}
	}

	allDepsWQ := allDeps
	allDepsWQ = append(allDepsWQ, queueDR)

	options := []*definition.DependencyOption{
		definition.NewDependancyOption("default", util.RandomString(DefaultRandomStringLength), allDeps),
		definition.NewDependancyOption("natsQ", util.RandomString(DefaultRandomStringLength), allDepsWQ),
	}

	frametests.WithTestDependencies(t, options, testFn)
}
