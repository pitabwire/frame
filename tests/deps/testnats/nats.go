package testnats

import (
	"context"
	"fmt"
	"net"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcNats "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests/testdef"
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
	image    string
	username string
	password string
	cluster  string

	conn         frame.DataSource
	internalConn frame.DataSource

	container *tcNats.NATSContainer
}

func NewNatsDep() testdef.TestResource {
	return NewNatsDepWithCred(NatsImage, NatsUser, NatsPass, NatsCluster)
}

func NewNatsDepWithCred(natsImage, natsUserName, natsPassword, cluster string) testdef.TestResource {
	return &natsDependancy{
		image:    natsImage,
		username: natsUserName,
		password: natsPassword,
		cluster:  cluster,
	}
}

func (d *natsDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *natsDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	natsqContainer, err := tcNats.Run(ctx, d.image,
		testcontainers.WithCmdArgs("--js", "-DVV"),
		tcNats.WithUsername(d.username),
		tcNats.WithPassword(d.password),

		network.WithNetwork([]string{ntwk.Name}, ntwk),
	)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}

	conn, err := natsqContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for container: %w", err)
	}

	d.conn = frame.DataSource(conn)

	internalIP, err := natsqContainer.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for container: %w", err)
	}
	d.internalConn = frame.DataSource(fmt.Sprintf("nats://%s", net.JoinHostPort(internalIP, "4222")))

	d.container = natsqContainer
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
