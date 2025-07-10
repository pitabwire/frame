package testnats

import (
	"context"
	"fmt"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcNats "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests/testdef"
)

const (

	// NATS configuration.

	NatsImage = "nats-streaming:0.25.5"

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

	conn frame.DataSource

	natsContainer *tcNats.NATSContainer
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
func (nd *natsDependancy) Setup(ctx context.Context, _ *testcontainers.DockerNetwork) error {
	natsqContainer, err := tcNats.Run(ctx, nd.image,
		tcNats.WithUsername(nd.username),
		tcNats.WithPassword(nd.password),
	)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}
	conn, err := natsqContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for postgres container: %w", err)
	}

	nd.conn = frame.DataSource(conn)

	nd.natsContainer = natsqContainer
	return nil
}

func (nd *natsDependancy) GetDS() frame.DataSource {
	return nd.conn
}

func (nd *natsDependancy) GetRandomisedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return nd.conn, func(_ context.Context) {
	}, nil
}

func (nd *natsDependancy) Cleanup(ctx context.Context) {
	if nd.natsContainer != nil {
		if err := nd.natsContainer.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate nats container")
		}
	}
}
