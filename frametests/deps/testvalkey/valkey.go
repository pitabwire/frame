package nats

import (
	"context"
	"fmt"
	"net"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcValKey "github.com/testcontainers/testcontainers-go/modules/valkey"
)

const (

	// ValKey configuration.

	ValKeyImage = "docker.io/valkey/valkey:latest"
	ValKeyPort  = "6379"

	ValKeyUser    = "frame"
	ValKeyPass    = "fr@m3"
	ValKeyCluster = "frame_test"
)

type valKeyDependancy struct {
	opts    definition.ContainerOpts
	cluster string

	conn         frame.DataSource
	internalConn frame.DataSource

	container *tcValKey.ValkeyContainer
}

func New() definition.TestResource {
	return NewWithOpts(ValKeyCluster)
}

func NewWithOpts(cluster string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      ValKeyImage,
		UserName:       ValKeyUser,
		Password:       ValKeyPass,
		Port:           ValKeyPort,
		NetworkAliases: []string{"valkey", "cache-valkey"},
		UseHostMode:    false,
		EnableLogging:  true,
	}
	opts.Setup(containerOpts...)

	return &valKeyDependancy{
		cluster: cluster,
		opts:    opts,
	}
}

func (d *valKeyDependancy) Name() string {
	return d.opts.ImageName
}

func (d *valKeyDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *valKeyDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {

	containerCustomize := d.opts.ConfigurationExtend(ctx, ntwk)
	valkeyContainer, err := tcValKey.Run(ctx, d.opts.ImageName, containerCustomize...)

	if err != nil {
		return fmt.Errorf("failed to start valkeyContainer: %w", err)
	}

	conn, err := valkeyContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for valkeyContainer: %w", err)
	}

	d.conn = frame.DataSource(conn)

	internalIP, err := valkeyContainer.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for valkeyContainer: %w", err)
	}
	d.internalConn = frame.DataSource(fmt.Sprintf("redis://%s", net.JoinHostPort(internalIP, d.opts.Port)))
	d.container = valkeyContainer
	return nil
}

func (d *valKeyDependancy) GetDS() frame.DataSource {
	return d.conn
}

func (d *valKeyDependancy) GetInternalDS() frame.DataSource {
	return d.conn
}

func (d *valKeyDependancy) GetRandomisedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(), func(_ context.Context) {
	}, nil
}

func (d *valKeyDependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate valkey container")
		}
	}
}
