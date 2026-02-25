package testnats

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	tcNats "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests/definition"
)

const (
	NatsImage = "nats:latest"

	NatsUser    = "frame"
	NatsPass    = "fr@m3"
	NatsCluster = "frame_test"
)

type dependancy struct {
	*definition.DefaultImpl
	cluster string
}

func New() definition.TestResource {
	return NewWithOpts(NatsCluster)
}

func NewWithOpts(cluster string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      NatsImage,
		UserName:       NatsUser,
		Credential:     NatsPass,
		Ports:          []string{"4222/tcp", "6222/tcp", "8222/tcp"},
		NetworkAliases: []string{"nats", "queue-nats"},
	}
	opts.Setup(containerOpts...)

	return &dependancy{
		DefaultImpl: definition.NewDefaultImpl(opts, "nats"),
		cluster:     cluster,
	}
}

func (d *dependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	containerCustomize := d.ConfigurationExtend(ctx, ntwk, []testcontainers.ContainerCustomizer{

		testcontainers.WithCmdArgs("--js", "-DVV"),
		tcNats.WithUsername(d.Opts().UserName),
		tcNats.WithPassword(d.Opts().Credential),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Server is ready")),
	}...)

	natsContainer, err := tcNats.Run(ctx, d.Name(), containerCustomize...)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}

	d.SetContainer(natsContainer)

	return nil
}

func (d *dependancy) GetDS(ctx context.Context) data.DSN {
	ds := d.DefaultImpl.GetDS(ctx)

	ds, _ = ds.WithUser(d.Opts().UserName)
	ds, _ = ds.WithPassword(d.Opts().Credential)
	return ds
}

func (d *dependancy) GetInternalDS(ctx context.Context) data.DSN {
	ds := d.DefaultImpl.GetInternalDS(ctx)

	ds, _ = ds.WithUser(d.Opts().UserName)
	ds, _ = ds.WithPassword(d.Opts().Credential)
	return ds
}
