package nats

import (
	"context"
	"fmt"
	"net"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcValKey "github.com/testcontainers/testcontainers-go/modules/valkey"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/testdef"
)

const (

	// ValKey configuration.

	ValKeyImage = "docker.io/valkey/valkey:8.1"

	ValKeyUser    = "frame"
	ValKeyPass    = "fr@m3"
	ValKeyCluster = "frame_test"
)

type valKeyDependancy struct {
	image    string
	username string
	password string
	cluster  string

	conn         frame.DataSource
	internalConn frame.DataSource

	container *tcValKey.ValkeyContainer
}

func NewValKeyDep() testdef.TestResource {
	return NewValKeyDepWithCred(ValKeyImage, ValKeyUser, ValKeyPass, ValKeyCluster)
}

func NewValKeyDepWithCred(image, userName, password, cluster string) testdef.TestResource {
	return &valKeyDependancy{
		image:    image,
		username: userName,
		password: password,
		cluster:  cluster,
	}
}

func (d *valKeyDependancy) Name() string {
	return d.image
}

func (d *valKeyDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *valKeyDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	container, err := tcValKey.Run(ctx, d.image, network.WithNetwork([]string{ntwk.Name}, ntwk))
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	conn, err := container.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for container: %w", err)
	}

	d.conn = frame.DataSource(conn)

	internalIP, err := container.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for container: %w", err)
	}
	d.internalConn = frame.DataSource(fmt.Sprintf("redis://%s", net.JoinHostPort(internalIP, "6379")))
	d.container = container
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
