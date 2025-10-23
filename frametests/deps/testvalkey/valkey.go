package nats

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	tcValKey "github.com/testcontainers/testcontainers-go/modules/valkey"

	"github.com/pitabwire/frame/frametests/definition"
)

const (

	// ValKey configuration.

	ValKeyImage = "docker.io/valkey/valkey:latest"

	ValKeyUser    = "frame"
	ValKeyPass    = "fr@m3"
	ValKeyCluster = "frame_test"
)

type valKeyDependancy struct {
	*definition.DefaultImpl
	cluster string
}

func New() definition.TestResource {
	return NewWithOpts(ValKeyCluster)
}

func NewWithOpts(cluster string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      ValKeyImage,
		UserName:       ValKeyUser,
		Password:       ValKeyPass,
		Ports:          []string{"6379/tcp"},
		NetworkAliases: []string{"valkey", "cache-valkey"},
	}
	opts.Setup(containerOpts...)

	return &valKeyDependancy{
		DefaultImpl: definition.NewDefaultImpl(opts, "redis"),
		cluster:     cluster,
	}
}

func (d *valKeyDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	containerCustomize := d.ConfigurationExtend(ctx, ntwk)
	valkeyContainer, err := tcValKey.Run(ctx, d.Name(), containerCustomize...)

	if err != nil {
		return fmt.Errorf("failed to start valkeyContainer: %w", err)
	}

	d.SetContainer(valkeyContainer)
	return nil
}
