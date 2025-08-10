package testnats

import (
	"context"
	"fmt"
	"net"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcNats "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
)

const (

	// NATS configuration.

	NatsImage = "nats:latest"

	NatsPort    = "4222"
	NatsUser    = "frame"
	NatsPass    = "fr@m3"
	NatsCluster = "frame_test"
)

type natsDependancy struct {
	opts    definition.ContainerOpts
	cluster string

	conn         frame.DataSource
	internalConn frame.DataSource

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
		Ports:          []string{NatsPort},
		NetworkAliases: []string{"nats", "queue-nats"},
	}
	opts.Setup(containerOpts...)

	return &natsDependancy{
		cluster: cluster,
		opts:    opts,
	}
}

func (d *natsDependancy) Name() string {
	return d.opts.ImageName
}
func (d *natsDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *natsDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	containerCustomize := d.opts.ConfigurationExtend(ctx, ntwk, []testcontainers.ContainerCustomizer{

		testcontainers.WithCmdArgs("--js", "-DVV"),
		tcNats.WithUsername(d.opts.UserName),
		tcNats.WithPassword(d.opts.Password),
	}...)

	natsqContainer, err := tcNats.Run(ctx, d.opts.ImageName, containerCustomize...)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}

	d.container = natsqContainer

	conn, err := natsqContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for container: %w", err)
	}

	d.conn = frame.DataSource(conn)

	internalIP, err := natsqContainer.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for container: %w", err)
	}
	d.internalConn = frame.DataSource(fmt.Sprintf("nats://%s", net.JoinHostPort(internalIP, d.opts.Ports[0])))

	return nil
}

func (d *natsDependancy) GetDS() frame.DataSource {
	return d.conn
}
func (d *natsDependancy) GetInternalDS() frame.DataSource {
	return d.internalConn
}

func (d *natsDependancy) GetRandomisedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(), func(_ context.Context) {
	}, nil
}

func (d *natsDependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate nats container")
		}
	}
}
