package queue

import (
	"context"
	"fmt"

	"github.com/pitabwire/util"
	tcNats "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests"
)

const (

	// NATS configuration.

	NatsImage = "nats-streaming:0.25.5"

	NatsPort    = "4222"
	NatsUser    = "frame"
	NatsPass    = "fr@m3"
	NatsCluster = "frame_test"
)

type NatsDependancy struct {
	image    string
	username string
	password string
	cluster  string

	conn frame.DataSource

	natsContainer *tcNats.NATSContainer
}

func NewNatsDep() tests.Dependancy {
	return NewNatsDepWithCred(NatsImage, NatsUser, NatsPass, NatsCluster)
}

func NewNatsDepWithCred(natsImage, natsUserName, natsPassword, cluster string) tests.Dependancy {
	return &NatsDependancy{
		image:    natsImage,
		username: natsUserName,
		password: natsPassword,
		cluster:  cluster,
	}
}
func (pg *NatsDependancy) Setup(ctx context.Context) error {
	natsqContainer, err := tcNats.Run(ctx, pg.image,
		tcNats.WithUsername(pg.username),
		tcNats.WithPassword(pg.password),
	)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}
	conn, err := natsqContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for postgres container: %w", err)
	}

	pg.conn = frame.DataSource(conn)

	pg.natsContainer = natsqContainer
	return nil
}

func (pg *NatsDependancy) GetDS() frame.DataSource {
	return pg.conn
}

func (pg *NatsDependancy) GetPrefixedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return pg.conn, func(_ context.Context) {
	}, nil
}

func (pg *NatsDependancy) Cleanup(ctx context.Context) {
	if pg.natsContainer != nil {
		if err := pg.natsContainer.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.Error("Failed to terminate nats container", "error", err)
		}
	}
}
