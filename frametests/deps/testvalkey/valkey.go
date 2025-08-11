package nats

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/docker/go-connections/nat"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcValKey "github.com/testcontainers/testcontainers-go/modules/valkey"

	"github.com/pitabwire/frame"
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
	opts    definition.ContainerOpts
	cluster string

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
		Ports:          []string{"6379/tcp"},
		NetworkAliases: []string{"valkey", "cache-valkey"},
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

	d.container = valkeyContainer
	return nil
}

func (d *valKeyDependancy) GetDS(ctx context.Context) frame.DataSource {
	port := nat.Port(d.opts.Ports[0])
	conn, err := d.container.PortEndpoint(ctx, port, "redis")
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get connection for Container")
	}

	return frame.DataSource(conn)
}

func (d *valKeyDependancy) GetInternalDS(ctx context.Context) frame.DataSource {
	internalIP, err := d.container.ContainerIP(ctx)
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get internal host ip for Container")
		return ""
	}

	if internalIP == "" && d.opts.UseHostMode {
		internalIP, err = d.container.Host(ctx)
		if err != nil {
			logger := util.Log(ctx).WithField("image", d.opts.ImageName)
			logger.WithError(err).Error("failed to get host ip for Container")
			return ""
		}
	}
	port := nat.Port(d.opts.Ports[0])

	return frame.DataSource(fmt.Sprintf("redis://%s", net.JoinHostPort(internalIP, strconv.Itoa(port.Int()))))
}

func (d *valKeyDependancy) GetRandomisedDS(
	ctx context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(ctx), func(_ context.Context) {}, nil
}

func (d *valKeyDependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate valkey container")
		}
	}
}
