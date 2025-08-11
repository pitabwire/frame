package testnats

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/docker/go-connections/nat"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcNats "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
)

const (
	NatsImage = "nats:latest"

	NatsUser    = "frame"
	NatsPass    = "fr@m3"
	NatsCluster = "frame_test"
)

type dependancy struct {
	opts    definition.ContainerOpts
	cluster string

	container *tcNats.NATSContainer
}

func New() definition.TestResource {
	return NewWithOpts(NatsCluster)
}

func NewWithOpts(cluster string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      NatsImage,
		UserName:       NatsUser,
		Password:       NatsPass,
		Ports:          []string{"4222/tcp", "6222/tcp", "8222/tcp"},
		NetworkAliases: []string{"nats", "queue-nats"},
	}
	opts.Setup(containerOpts...)

	return &dependancy{
		cluster: cluster,
		opts:    opts,
	}
}

func (d *dependancy) Name() string {
	return d.opts.ImageName
}
func (d *dependancy) Container() testcontainers.Container {
	return d.container
}

func (d *dependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	containerCustomize := d.opts.ConfigurationExtend(ctx, ntwk, []testcontainers.ContainerCustomizer{

		testcontainers.WithCmdArgs("--js", "-DVV"),
		tcNats.WithUsername(d.opts.UserName),
		tcNats.WithPassword(d.opts.Password),
	}...)

	natsContainer, err := tcNats.Run(ctx, d.opts.ImageName, containerCustomize...)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}

	d.container = natsContainer

	return nil
}

func (d *dependancy) GetDS(ctx context.Context) frame.DataSource {
	port := nat.Port(d.opts.Ports[0])
	conn, err := d.container.PortEndpoint(ctx, port, "nats")
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get connection for Container")
	}

	return frame.DataSource(conn)
}

func (d *dependancy) GetInternalDS(ctx context.Context) frame.DataSource {
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

	return frame.DataSource(fmt.Sprintf("nats://%s", net.JoinHostPort(internalIP, strconv.Itoa(port.Int()))))
}

func (d *dependancy) GetRandomisedDS(
	ctx context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(ctx), func(_ context.Context) {
	}, nil
}

func (d *dependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate nats container")
		}
	}
}
